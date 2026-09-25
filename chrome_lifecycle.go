package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// getBrowser returns the current browser under a read lock.
func (m *ChromeManager) getBrowser() *rod.Browser {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.browser
}

// getGuard returns the current connection's egress guard under a read lock
// — reconnect() replaces both m.browser and m.guard together, so this must
// use the same locking discipline as getBrowser to avoid handing a caller a
// guard from a since-replaced connection.
func (m *ChromeManager) getGuard() *egressGuard {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.guard
}

// startDisconnectWatcher monitors the CDP connection for unexpected disconnects.
// When the browser's WebSocket closes (Chrome crash, network drop), rod's
// EachEvent goroutine exits. We detect this and close LostConnection unless
// Close() has already closed closingGracefully. chromedp pattern.
func (m *ChromeManager) startDisconnectWatcher() {
	go func() {
		b := m.getBrowser()
		if b == nil {
			return
		}
		// Wait for the browser's event loop to exit — this happens when the
		// CDP WebSocket closes. rod doesn't expose a direct "disconnected" channel,
		// but we can poll Connected() or use a heartbeat.
		// Simpler: use a ticker to poll browser connectivity every 5s.
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-m.closingGracefully:
				return
			case <-ticker.C:
				if !m.Connected() {
					// Connection lost — check if it's intentional.
					select {
					case <-m.closingGracefully:
						return // intentional close, don't fire LostConnection
					default:
					}
					select {
					case <-m.LostConnection:
						// already closed
					default:
						close(m.LostConnection)
					}
					return
				}
				// Active health probe: try a no-op CDP call.
				b := m.getBrowser()
				if b == nil {
					continue
				}
				if _, err := (&proto.BrowserGetVersion{}).Call(b); err != nil {
					select {
					case <-m.closingGracefully:
						return
					default:
					}
					select {
					case <-m.LostConnection:
					default:
						close(m.LostConnection)
					}
					return
				}
			}
		}
	}()
}

// reconnect closes the old connection and establishes a new one.
// The slow CDP work (discover, connect, install guard) runs WITHOUT holding m.mu
// so concurrent getBrowser()/getGuard() callers are not blocked and never see a
// nil browser during the reconnect window. The old browser stays readable until
// the new one is ready, then we atomically swap both browser and guard under lock.
func (m *ChromeManager) reconnect() error {
	// Serialize reconnect — only one at a time.
	m.reconnectMu.Lock()
	defer m.reconnectMu.Unlock()

	// Snapshot the old browser under lock, then release the lock for the slow work.
	m.mu.RLock()
	oldBrowser := m.browser
	m.mu.RUnlock()

	// Close the old browser outside the lock — may take seconds.
	if oldBrowser != nil {
		_ = oldBrowser.Close()
	}

	debuggerURL, err := discoverWSURL(m.wsURL)
	if err != nil {
		return fmt.Errorf("discover ws url: %w", err)
	}

	b := rod.New().ControlURL(debuggerURL)
	if err := b.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// Re-install the egress guard on the fresh connection — it does NOT
	// carry over from the closed browser (see egress_guard.go). Fail the
	// reconnect rather than resume operation unguarded.
	guard, err := installEgressGuard(b)
	if err != nil {
		_ = b.Close()
		return fmt.Errorf("%w", err)
	}

	// Atomically swap browser + guard under lock. Callers never see nil.
	m.mu.Lock()
	m.browser = b
	m.guard = guard
	m.keepaliveCtxID = ""
	pool := m.pool
	// Recreate LostConnection for the new connection — the old one may have been closed.
	m.LostConnection = make(chan struct{})
	m.mu.Unlock()

	if pool != nil {
		pool.UpdateBrowser(b)
	} else {
		// No pool yet — create one. This is the only path that creates a new pool.
		m.mu.Lock()
		if m.pool == nil {
			m.pool = NewContextPool(b)
		}
		m.mu.Unlock()
	}
	// Restart the disconnect watcher on the new connection.
	m.startDisconnectWatcher()
	return nil
}

// Connected reports whether the Chrome connection is active.
func (m *ChromeManager) Connected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.browser != nil
}

// HealthStatus is the result of a ChromeManager health probe.
// #49: Returned by HealthCheck() for the /health endpoint and external monitors.
type HealthStatus struct {
	Connected bool   `json:"connected"`
	WsURL     string `json:"ws_url"`
	LatencyMs int64  `json:"latency_ms"`
	// Targets is the number of CDP targets Chrome reported during the probe.
	Targets     int        `json:"targets,omitempty"`
	ContextPool *PoolStats `json:"context_pool,omitempty"`
}

// healthProbeTimeout bounds every CDP probe inside HealthCheck — the probe
// exists to detect a wedged browser, so it must never wait unboundedly.
const healthProbeTimeout = 2 * time.Second

// PoolStats is a snapshot of the ContextPool state for health reporting.
type PoolStats struct {
	Contexts   int    `json:"contexts"`
	Pages      int    `json:"pages"`
	Generation uint64 `json:"generation"`
}

// HealthCheck performs an active CDP health probe and returns the status.
// If the browser is not connected, returns a disconnected status without
// attempting a CDP call (which would block).
func (m *ChromeManager) HealthCheck() HealthStatus {
	m.mu.RLock()
	b := m.browser
	m.mu.RUnlock()

	status := HealthStatus{
		Connected: b != nil,
		WsURL:     m.wsURL,
	}

	if b == nil {
		return status
	}

	// All probes share one bounded ctx — an unbounded Call on a wedged
	// browser would hang the health endpoint itself (socket alive, browser
	// dead — the exact failure this probe exists to catch).
	probeCtx, cancel := context.WithTimeout(context.Background(), healthProbeTimeout)
	defer cancel()
	hb := b.Context(probeCtx)

	// Active health probe: measure CDP round-trip latency.
	start := time.Now()
	if _, err := (&proto.BrowserGetVersion{}).Call(hb); err != nil {
		// CDP call failed — connection is stale even though browser != nil.
		status.Connected = false
		return status
	}
	status.LatencyMs = time.Since(start).Milliseconds()

	// Page-level probe (krolik-server#493): BrowserGetVersion proves the
	// browser domain answers, but a wedged target set — e.g. after an
	// external Chrome restart behind a persistent websocket — leaves every
	// pooled tab dead while the socket stays healthy. TargetGetTargets
	// forces the browser to enumerate its targets; failure => unhealthy.
	targets, err := (&proto.TargetGetTargets{}).Call(hb)
	if err != nil {
		status.Connected = false
		return status
	}
	status.Targets = len(targets.TargetInfos)

	// Gather pool stats if available.
	if pool := m.Pool(); pool != nil {
		pool.contextsMu.RLock()
		ctxCount := len(pool.contexts)
		pageCount := 0
		var probeTarget proto.TargetTargetID
		for _, mc := range pool.contexts {
			mc.Mu.Lock()
			pageCount += len(mc.Pages)
			if probeTarget == "" {
				for _, mp := range mc.Pages {
					if mp.Page != nil {
						probeTarget = mp.Page.TargetID
						break
					}
				}
			}
			mc.Mu.Unlock()
		}
		pool.contextsMu.RUnlock()
		status.ContextPool = &PoolStats{
			Contexts:   ctxCount,
			Pages:      pageCount,
			Generation: pool.generation.Load(),
		}
		// If the pool holds pages, verify one actually resolves in the
		// browser — a pool full of destroyed targets is dead even when the
		// connection is alive.
		if probeTarget != "" {
			if _, err := (proto.TargetGetTargetInfo{TargetID: probeTarget}).Call(hb); err != nil {
				status.Connected = false
				return status
			}
		}
	}

	return status
}

// Close disconnects from Chrome and releases resources.
// Signals closingGracefully so the disconnect watcher doesn't fire LostConnection.
// Sets browser to nil under lock so concurrent callers see the shutdown state,
// then closes the old browser outside the lock (may take seconds).
func (m *ChromeManager) Close() {
	// Signal intentional close — disconnect watcher checks this before firing LostConnection.
	select {
	case <-m.closingGracefully:
	default:
		close(m.closingGracefully)
	}

	m.mu.Lock()
	b := m.browser
	m.browser = nil
	m.guard = nil
	ctxID := m.keepaliveCtxID
	m.keepaliveCtxID = ""
	m.mu.Unlock()

	if b != nil {
		if ctxID != "" {
			_ = proto.TargetDisposeBrowserContext{BrowserContextID: ctxID}.Call(b)
		}
		_ = b.Close()
	}
}

// DefaultContext returns the browser's default context (persistent profile).
// Cookies, localStorage, and other state from manual login sessions are available.
// No isolation — all requests share the same cookies.
func (m *ChromeManager) DefaultContext() (*rod.Browser, error) {
	b := m.getBrowser()
	if b == nil {
		return nil, ErrUnavailable
	}
	scoped := b.NoDefaultDevice()
	scoped.BrowserContextID = "" // empty = default context (persistent profile)
	return scoped, nil
}

// PageInfo describes an open browser page/tab.
type PageInfo struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Context string `json:"context"` // "default", "keepalive", or context ID
}

// ListPages returns all open pages/tabs with their context info.
func (m *ChromeManager) ListPages() ([]PageInfo, error) {
	b := m.getBrowser()
	if b == nil {
		return nil, ErrUnavailable
	}
	targets, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	var pages []PageInfo
	for _, t := range targets.TargetInfos {
		if t.Type != "page" {
			continue
		}
		ctx := "default"
		if t.BrowserContextID != "" {
			if t.BrowserContextID == m.keepaliveCtxID {
				ctx = "keepalive"
			} else {
				ctx = string(t.BrowserContextID)
			}
		}
		pages = append(pages, PageInfo{
			ID:      string(t.TargetID),
			Type:    string(t.Type),
			URL:     t.URL,
			Title:   t.Title,
			Context: ctx,
		})
	}
	return pages, nil
}
