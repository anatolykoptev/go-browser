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

// NewContextPool creates a ContextPool, starts the background reaper,
// and subscribes to CDP target destruction events.
func NewContextPool(browser *rod.Browser) *ContextPool {
	p := &ContextPool{
		browser:  browser,
		contexts: make(map[string]*ManagedContext),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go p.reaper()
	p.watchTargetDestroyed()
	return p
}

// GetOrCreatePage returns the existing page for session, or creates a new tab in the
// appropriate context. If session is empty an ephemeral name is generated.
func (p *ContextPool) GetOrCreatePage(session, mode, proxy, url string) (*ManagedPage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := contextKey(mode, proxy)

	mc, err := p.getOrCreateContext(key, mode, proxy)
	if err != nil {
		return nil, err
	}

	// Look up existing session.
	if mp, ok := mc.Pages[session]; ok {
		mp.LastUsed = time.Now()
		if url != "" && url != "about:blank" && url != mp.URL {
			mp.URL = url
		}
		return mp, nil
	}

	// Deduplicate auto-generated names (e.g. "example.com" → "example.com-2").
	session = deduplicateSession(mc, session)

	// For default context with no pages yet, adopt Chrome's existing default tab.
	if key == "default" && len(mc.Pages) == 0 {
		if adopted, err := p.adoptExistingPage(mc); err == nil && adopted != nil {
			mp := &ManagedPage{
				Session:  session,
				Page:     adopted,
				URL:      url,
				LastUsed: time.Now(),
				TTL:      contextPoolDefaultTTL,
				Refs:     NewRefMap(),
			}
			mc.Pages[session] = mp
			return mp, nil
		}
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
