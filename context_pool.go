package browser

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const sessionIDBytes = 16

// generateID creates a random hex session ID.
func generateID() (string, error) {
	b := make([]byte, sessionIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

const (
	contextPoolReaperInterval = 30 * time.Second
	contextPoolDefaultTTL     = 30 * time.Minute
)

// ContextPool manages named browser sessions grouped by context (default/private/proxy).
// Each context maps to a Chrome BrowserContext; each session is a named tab within it.
type ContextPool struct {
	mu       sync.Mutex
	browser  *rod.Browser
	contexts map[string]*ManagedContext // key: "default" | "private" | "proxy:<url>"
	stop     chan struct{}
	done     chan struct{}
}

// ManagedContext is a Chrome BrowserContext with a set of named pages.
type ManagedContext struct {
	ID    proto.BrowserBrowserContextID
	Mode  string // "default", "private", "proxy"
	Proxy string // proxy URL (only for mode=proxy)
	Pages map[string]*ManagedPage
}

// ManagedPage is a named tab within a ManagedContext.
type ManagedPage struct {
	Session  string
	Page     *rod.Page
	URL      string
	LastUsed time.Time
	TTL      time.Duration // 0 = never expires
	Refs     *RefMap
}

// ContextInfo describes a context and its sessions (for chrome_tabs tool).
type ContextInfo struct {
	Mode     string        `json:"mode"`
	Proxy    string        `json:"proxy,omitempty"`
	Sessions []SessionInfo `json:"sessions"`
}

// SessionInfo describes a single named session.
type SessionInfo struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	LastUsed string `json:"last_used"`
}

// NewContextPool creates a ContextPool and starts the background reaper.
func NewContextPool(browser *rod.Browser) *ContextPool {
	p := &ContextPool{
		browser:  browser,
		contexts: make(map[string]*ManagedContext),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go p.reaper()
	return p
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

// GetOrCreatePage returns the existing page for session, or creates a new tab in the
// appropriate context. If session is empty an ephemeral name is generated.
func (p *ContextPool) GetOrCreatePage(session, mode, proxy, url string) (*ManagedPage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := contextKey(mode, proxy)

	// Get or create the context.
	mc, err := p.getOrCreateContext(key, mode, proxy)
	if err != nil {
		return nil, err
	}

	// Look up existing session.
	if mp, ok := mc.Pages[session]; ok {
		mp.LastUsed = time.Now()
		// Navigate if URL changed and URL is not empty.
		if url != "" && url != "about:blank" && url != mp.URL {
			mp.URL = url
		}
		return mp, nil
	}

	// Create new tab in this context.
	page, err := p.newPageInContext(mc)
	if err != nil {
		return nil, fmt.Errorf("context_pool: create tab in context %q: %w", key, err)
	}

	mp := &ManagedPage{
		Session:  session,
		Page:     page,
		URL:      url,
		LastUsed: time.Now(),
		TTL:      contextPoolDefaultTTL,
		Refs:     NewRefMap(),
	}
	mc.Pages[session] = mp
	return mp, nil
}

// ClosePage closes a specific session's page. Disposes context if no pages remain.
// The default context is never disposed.
func (p *ContextPool) ClosePage(session string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, mc := range p.contexts {
		mp, ok := mc.Pages[session]
		if !ok {
			continue
		}
		if mp.Page != nil {
			_ = mp.Page.Close()
		}
		delete(mc.Pages, session)
		if len(mc.Pages) == 0 && key != "default" {
			p.disposeContext(mc)
			delete(p.contexts, key)
		}
		return nil
	}
	return fmt.Errorf("context_pool: session %q not found", session)
}

// SessionCount returns the total number of active named sessions across all contexts.
func (p *ContextPool) SessionCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, mc := range p.contexts {
		n += len(mc.Pages)
	}
	return n
}

// List returns all contexts and their sessions.
func (p *ContextPool) List() []ContextInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]ContextInfo, 0, len(p.contexts))
	for _, mc := range p.contexts {
		ci := ContextInfo{
			Mode:  mc.Mode,
			Proxy: mc.Proxy,
		}
		for _, mp := range mc.Pages {
			ci.Sessions = append(ci.Sessions, SessionInfo{
				Name:     mp.Session,
				URL:      mp.URL,
				LastUsed: formatAge(mp.LastUsed),
			})
		}
		result = append(result, ci)
	}
	return result
}

// Reap closes expired pages and disposes empty non-default contexts.
func (p *ContextPool) Reap() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, mc := range p.contexts {
		for name, mp := range mc.Pages {
			if mp.TTL > 0 && time.Since(mp.LastUsed) > mp.TTL {
				if mp.Page != nil {
					_ = mp.Page.Close()
				}
				delete(mc.Pages, name)
			}
		}
		if len(mc.Pages) == 0 && key != "default" {
			p.disposeContext(mc)
			delete(p.contexts, key)
		}
	}
}

// Close stops the reaper goroutine. Does not close pages.
func (p *ContextPool) Close() {
	select {
	case <-p.stop:
	default:
		close(p.stop)
	}
	<-p.done
}

// UpdateBrowser replaces the browser reference (after reconnect).
func (p *ContextPool) UpdateBrowser(b *rod.Browser) {
	p.mu.Lock()
	p.browser = b
	p.mu.Unlock()
}

// --- internal helpers ---

func (p *ContextPool) getOrCreateContext(key, mode, proxy string) (*ManagedContext, error) {
	if mc, ok := p.contexts[key]; ok {
		return mc, nil
	}

	mc := &ManagedContext{
		Mode:  mode,
		Proxy: proxy,
		Pages: make(map[string]*ManagedPage),
	}

	if mode == "default" {
		// Default context uses empty BrowserContextID — Chrome's persistent profile.
		p.contexts[key] = mc
		return mc, nil
	}

	// Create new incognito/proxy BrowserContext.
	proxyServer, _, _ := parseProxy(proxy)
	res, err := proto.TargetCreateBrowserContext{
		ProxyServer:     proxyServer,
		DisposeOnDetach: true,
	}.Call(p.browser)
	if err != nil {
		return nil, fmt.Errorf("create browser context: %w", err)
	}
	mc.ID = res.BrowserContextID
	p.contexts[key] = mc
	return mc, nil
}

func (p *ContextPool) newPageInContext(mc *ManagedContext) (*rod.Page, error) {
	var targetReq proto.TargetCreateTarget
	targetReq.URL = "about:blank"
	if mc.ID != "" {
		targetReq.BrowserContextID = mc.ID
	}
	res, err := targetReq.Call(p.browser)
	if err != nil {
		return nil, fmt.Errorf("create target: %w", err)
	}
	page, err := p.browser.PageFromTarget(res.TargetID)
	if err != nil {
		return nil, fmt.Errorf("page from target: %w", err)
	}
	return page, nil
}

func (p *ContextPool) disposeContext(mc *ManagedContext) {
	if mc.ID != "" {
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: mc.ID}.Call(p.browser)
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
