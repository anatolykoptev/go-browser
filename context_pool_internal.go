package browser

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

const pageCloseTimeout = 5 * time.Second

// closePageWithTimeout calls page.Close() under a 5s timeout so a hung target
// cannot block the reaper or ClosePage caller indefinitely.
func closePageWithTimeout(page *rod.Page) {
	if page == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = page.Close()
	}()
	select {
	case <-done:
	case <-time.After(pageCloseTimeout):
		// Give up — the target is stuck. Chrome will eventually GC it.
	}
}

// contextKey returns the map key for the given mode/proxy combination.
func contextKey(mode, proxy string) string {
	switch mode {
	case "default":
		return "default"
	case "proxy":
		return "proxy:" + proxy
	default:
		return "private"
	}
}

// knownIncognitoCtxIDs snapshots the BrowserContextIDs of every pool-created
// (non-"default") context — the private/proxy incognito contexts created via
// TargetCreateBrowserContext. The default context's ID is DISCOVERED (not
// created), so its key is deliberately excluded. Follows the pool's two-phase
// lock discipline: snapshot the context pointers under contextsMu, release it,
// then read each mc.ID under mc.Mu — never holding contextsMu across a mc.Mu
// acquisition, and never holding either across a CDP call.
func (p *ContextPool) knownIncognitoCtxIDs() map[proto.BrowserBrowserContextID]struct{} {
	p.contextsMu.RLock()
	ctxs := make([]*ManagedContext, 0, len(p.contexts))
	for key, mc := range p.contexts {
		if key == "default" {
			continue
		}
		ctxs = append(ctxs, mc)
	}
	p.contextsMu.RUnlock()

	ids := make(map[proto.BrowserBrowserContextID]struct{}, len(ctxs))
	for _, mc := range ctxs {
		mc.Mu.Lock()
		id := mc.ID
		mc.Mu.Unlock()
		// Invariant: a non-"default" context is only ever published into p.contexts
		// with a non-empty ID — getOrCreateContextSafe errors out if
		// TargetCreateBrowserContext fails and otherwise sets mc.ID to the returned
		// (non-empty) BrowserContextID before inserting, and only the default
		// context's ID is ever reset to "". So this guard never drops a real
		// incognito context; it only keeps "" out of the exclusion set (which would
		// otherwise wrongly exclude the empty-ID persistent-default target).
		if id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

// discoverPersistentDefaultCtxID reads the live page targets and returns the
// BrowserContextID of the persistent default context — the first page target
// that does NOT belong to a pool-created incognito (private/proxy) context.
//
// The pool creates private/proxy contexts (TargetCreateBrowserContext) that also
// carry live page targets, and TargetGetTargets returns targets in arbitrary
// order. A naive "first page target wins" scan could therefore bind the default
// context to an incognito BrowserContextID — landing every subsequent
// mode=default page in an anonymous incognito cookie jar instead of the
// persistent authenticated profile. Excluding the pool's own incognito contexts
// guarantees the discovered ID is the persistent default (e.g. the operator's
// ambient login tab).
//
// ORDER MATTERS — read the targets BEFORE snapshotting the incognito set. The
// pool inserts a private/proxy context into p.contexts (getOrCreateContextSafe)
// strictly BEFORE creating its first page target (GetOrCreatePage Phase 3,
// newPageInContext). So if TargetGetTargets observed a private page target, that
// context was already registered, and a snapshot taken AFTER the CDP call is
// guaranteed to include its ID → it is correctly excluded. Snapshotting FIRST
// would reopen the leak as a race window: a concurrent mode=private/proxy create
// running on go-wowa's scraping tabs could register its context and surface its
// page BETWEEN an earlier snapshot and this call, and that page would be misread
// as the persistent default. The only residual race under this order is a
// context disposed between the two calls — a dead id that fails the subsequent
// TargetCreateTarget with the stale-context error and self-heals to the empty-ID
// fallback (no live-jar leak; strictly safer).
//
// Returns "" when no non-incognito page target exists (or the CDP call fails):
// the caller then omits BrowserContextID on TargetCreateTarget and lands in
// Chrome's current default context — the documented empty-ID fallback.
func (p *ContextPool) discoverPersistentDefaultCtxID() proto.BrowserBrowserContextID {
	b := p.getBrowser()
	if b == nil {
		return ""
	}
	targets, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		return ""
	}
	// Snapshot AFTER the CDP read so any context whose page target we just
	// observed is guaranteed to be in the exclusion set (see ORDER MATTERS above).
	incognito := p.knownIncognitoCtxIDs()
	for _, t := range targets.TargetInfos {
		if t.Type != "page" {
			continue
		}
		if _, isIncognito := incognito[t.BrowserContextID]; isIncognito {
			continue
		}
		return t.BrowserContextID
	}
	return ""
}

// getOrCreateContextSafe does the full read→upgrade→write cycle for the
// contexts map, doing any CDP BrowserContext creation OUTSIDE the lock.
func (p *ContextPool) getOrCreateContextSafe(key, mode, proxy string) (*ManagedContext, error) {
	// Fast path: read lock.
	p.contextsMu.RLock()
	if mc, ok := p.contexts[key]; ok {
		p.contextsMu.RUnlock()
		return mc, nil
	}
	p.contextsMu.RUnlock()

	// Slow path: build the new context (CDP call happens here, unlocked).
	mc := &ManagedContext{Mode: mode, Proxy: proxy, Pages: make(map[string]*ManagedPage)}

	// For default mode, discover the default BrowserContextID from existing tabs
	// so that TargetCreateTarget creates a tab in the same window instead of a new
	// one. Pool-created private/proxy incognito contexts are excluded so a
	// mode=default page never binds to an incognito jar (see
	// discoverPersistentDefaultCtxID).
	if mode == "default" {
		mc.ID = p.discoverPersistentDefaultCtxID()
	}

	if mode != "default" {
		proxyServer, _, _ := parseProxy(proxy)
		b := p.getBrowser()
		if b == nil {
			return nil, ErrUnavailable
		}
		res, err := proto.TargetCreateBrowserContext{
			ProxyServer:     proxyServer,
			DisposeOnDetach: true,
		}.Call(b)
		if err != nil {
			return nil, fmt.Errorf("create browser context: %w", err)
		}
		mc.ID = res.BrowserContextID
	}

	p.contextsMu.Lock()
	defer p.contextsMu.Unlock()
	// Double-check: another goroutine may have created it while we were in CDP.
	if existing, ok := p.contexts[key]; ok {
		if mc.ID != "" && mode != "default" {
			if b := p.getBrowser(); b != nil {
				_ = proto.TargetDisposeBrowserContext{BrowserContextID: mc.ID}.Call(b)
			}
		}
		return existing, nil
	}
	p.contexts[key] = mc
	return mc, nil
}

func (p *ContextPool) newPageInContext(mc *ManagedContext) (*rod.Page, error) {
	b := p.getBrowser()
	if b == nil {
		return nil, ErrUnavailable
	}
	var targetReq proto.TargetCreateTarget
	targetReq.URL = "about:blank"
	// Snapshot mc.ID under mc.Mu: rediscoverDefaultContext may rewrite it
	// concurrently during stale-context recovery, and post-publication mc.ID
	// mutations are mc.Mu-guarded.
	mc.Mu.Lock()
	ctxID := mc.ID
	mc.Mu.Unlock()
	if ctxID != "" {
		targetReq.BrowserContextID = ctxID
	}
	res, err := targetReq.Call(b)
	if err != nil {
		return nil, fmt.Errorf("create target: %w", err)
	}
	page, err := b.PageFromTarget(res.TargetID)
	if err != nil {
		return nil, fmt.Errorf("page from target: %w", err)
	}
	return page, nil
}

// adoptExistingPage finds Chrome's existing default tab (e.g. chrome://newtab)
// and wraps it as a rod.Page so the pool can manage it instead of creating a new one.
func (p *ContextPool) adoptExistingPage(mc *ManagedContext) (*rod.Page, error) {
	b := p.getBrowser()
	if b == nil {
		return nil, ErrUnavailable
	}
	targets, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		return nil, err
	}
	// Snapshot mc.ID under mc.Mu — rediscoverDefaultContext may rewrite it
	// concurrently during stale-context recovery (mc.ID is mc.Mu-guarded).
	mc.Mu.Lock()
	ctxID := mc.ID
	mc.Mu.Unlock()
	for _, t := range targets.TargetInfos {
		if t.Type != "page" || t.BrowserContextID != ctxID {
			continue
		}
		page, err := b.PageFromTarget(t.TargetID)
		if err != nil {
			continue
		}
		return page, nil
	}
	// #31: If no page matched the cached ctxID, rediscover the default context
	// and try again. The cached mc.ID may be stale (Chrome disposed/recreated
	// the context). This matches the recovery in createPageWithStaleRecovery.
	if ctxID != p.discoverPersistentDefaultCtxID() {
		liveID := p.discoverPersistentDefaultCtxID()
		if liveID != "" {
			mc.Mu.Lock()
			mc.ID = liveID
			mc.Mu.Unlock()
			for _, t := range targets.TargetInfos {
				if t.Type != "page" || t.BrowserContextID != liveID {
					continue
				}
				page, err := b.PageFromTarget(t.TargetID)
				if err != nil {
					continue
				}
				return page, nil
			}
		}
	}
	return nil, fmt.Errorf("no existing page found")
}

func (p *ContextPool) disposeContext(mc *ManagedContext) {
	if mc.ID != "" {
		if b := p.getBrowser(); b != nil {
			_ = proto.TargetDisposeBrowserContext{BrowserContextID: mc.ID}.Call(b)
		}
	}
}

func (p *ContextPool) reaper() {
	defer close(p.done)
	ticker := time.NewTicker(contextPoolReaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.Reap()
		}
	}
}

// watchTargetDestroyed listens for CDP target destruction events and removes
// closed pages from the pool (e.g. when user closes a tab in VNC).
// Only removes pages whose TargetID exactly matches the destroyed target.
// Does NOT dispose the BrowserContext — let Chrome handle context lifecycle.
func (p *ContextPool) watchTargetDestroyed() {
	b := p.getBrowser()
	if b == nil {
		return
	}
	go b.EachEvent(func(e *proto.TargetTargetDestroyed) bool {
		p.contextsMu.RLock()
		ctxs := make([]*ManagedContext, 0, len(p.contexts))
		for _, mc := range p.contexts {
			ctxs = append(ctxs, mc)
		}
		p.contextsMu.RUnlock()
		for _, mc := range ctxs {
			mc.Mu.Lock()
			for name, mp := range mc.Pages {
				if mp.Page != nil && mp.Page.TargetID == e.TargetID {
					delete(mc.Pages, name)
					mc.Mu.Unlock()
					return false
				}
			}
			mc.Mu.Unlock()
		}
		return false
	})()
}

// sessionNameFromURL derives a human-readable session name from a URL.
// Example: "https://example.com/page?q=1" → "example.com/page".
// #20: Logs a warning when the URL is malformed or has no host — the session
// name silently downgrades to an ephemeral ID, which makes session management
// harder for callers expecting a deterministic name.
func sessionNameFromURL(rawURL string) string {
	if rawURL == "" || rawURL == "about:blank" {
		return generateEphemeralID()
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		slog.Warn("context_pool: malformed URL for session name — using ephemeral ID", "url", rawURL, "err", err)
		return generateEphemeralID()
	}
	if u.Host == "" {
		slog.Warn("context_pool: URL has no host for session name — using ephemeral ID", "url", rawURL)
		return generateEphemeralID()
	}
	name := u.Host + u.Path
	name = strings.TrimRight(name, "/")
	if name == "" {
		return u.Host
	}
	return name
}

// deduplicateSession appends -2, -3, etc. if the session name already exists in the context.
func deduplicateSession(mc *ManagedContext, base string) string {
	if _, exists := mc.Pages[base]; !exists {
		return base
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, exists := mc.Pages[candidate]; !exists {
			return candidate
		}
	}
	return base + "-" + generateEphemeralID()
}

// formatAge formats the duration since t as a short human-readable string.
func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// isStaleBrowserContextErr reports whether err is the CDP signal that a
// BrowserContextID no longer exists — i.e. the context was disposed/recreated
// out from under the pool. CDP returns error -32000 with the message
// "Failed to find browser context with id <ID>" (a Chromium-emitted string;
// go-rod has no typed sentinel for this class). This is the stale-default-
// context class that latched the pool forever until a process restart.
//
// It matches on the typed *cdp.Error (code -32000 + message) when available and
// falls back to a substring scan of the wrapped error chain so a future extra
// wrap layer cannot defeat detection.
func isStaleBrowserContextErr(err error) bool {
	if err == nil {
		return false
	}
	const marker = "Failed to find browser context"
	var cdpErr *cdp.Error
	if errors.As(err, &cdpErr) {
		return cdpErr.Code == -32000 && strings.Contains(cdpErr.Message, marker)
	}
	return strings.Contains(err.Error(), marker)
}

// rediscoverDefaultContext re-reads the live default BrowserContextID from the
// browser's current page targets and updates mc.ID in place. It is only
// meaningful for the default context, whose ID the pool discovers (rather than
// creates) and which Chrome may dispose/recreate independently.
//
// If a live persistent-default page target is found (pool-created private/proxy
// incognito contexts are excluded), mc.ID is set to its live BrowserContextID.
// If none is found, mc.ID is reset to empty so the subsequent TargetCreateTarget
// omits BrowserContextID and lands in Chrome's current default context. Either
// way the stale handle is cleared.
//
// Discovery skips page targets that belong to the pool's own incognito contexts
// (see discoverPersistentDefaultCtxID): binding the default context to an
// incognito BrowserContextID would land mode=default pages in an anonymous jar.
// The empty-ID result is the documented "land in current default" fallback. The
// write holds mc.Mu — post-publication mc.ID mutations must, because
// newPageInContext reads mc.ID under the same lock (see ctxID snapshot).
// Returns true if mc.ID changed.
func (p *ContextPool) rediscoverDefaultContext(mc *ManagedContext) bool {
	liveID := p.discoverPersistentDefaultCtxID()
	mc.Mu.Lock()
	changed := mc.ID != liveID
	mc.ID = liveID
	mc.Mu.Unlock()
	return changed
}

// createPageWithStaleRecovery creates a page in mc, recovering once from a stale
// default BrowserContextID. The default context's ID is DISCOVERED (not created)
// and Chrome may dispose/recreate it independently; when that happens the cached
// mc.ID goes stale and TargetCreateTarget fails with "Failed to find browser
// context with id ...", which previously latched the pool until a restart.
//
// Recovery is scoped to the default context only (key == "default"). This gate is
// a context-ISOLATION invariant, not merely an optimization: a private/proxy
// context owns a BrowserContext the pool created itself, and re-discovering the
// "live default" for it would silently rebind an isolated/proxied context onto
// the shared default context — a context-isolation / proxy-bypass leak. Gating on
// default keeps the change a no-op on the happy path and for non-default callers,
// AND preserves isolation.
func (p *ContextPool) createPageWithStaleRecovery(mc *ManagedContext, key string) (*rod.Page, error) {
	page, err := p.newPageInContext(mc)
	if err == nil || key != "default" || !isStaleBrowserContextErr(err) {
		return page, err
	}

	// Stale default-context handle observed — re-discover the live default context
	// and retry with a short backoff. The delay handles the case where Chrome is
	// in the middle of disposing/recreating the context (TOCTOU window).
	// #22: Added 200ms backoff before retry — was immediate, which could race
	// with Chrome's context recreation.
	recordStaleCtxRecovery(StaleCtxOutcomeDetected)
	if !p.rediscoverDefaultContext(mc) {
		recordStaleCtxRecovery(StaleCtxOutcomeFailed)
		return nil, err
	}
	// Brief backoff to let Chrome finish context recreation.
	time.Sleep(200 * time.Millisecond)
	page, err = p.newPageInContext(mc)
	if err != nil {
		recordStaleCtxRecovery(StaleCtxOutcomeFailed)
		return nil, err
	}
	recordStaleCtxRecovery(StaleCtxOutcomeRecovered)
	return page, nil
}
