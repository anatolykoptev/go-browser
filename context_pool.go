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
//
// Locking discipline:
//   - contextsMu (RWMutex) guards the contexts map itself.
//   - ManagedContext.Mu (Mutex) guards that context's Pages map.
//   - CDP I/O (TargetCreateTarget, Page.Close, etc.) runs OUTSIDE any lock.
type ContextPool struct {
	contextsMu sync.RWMutex
	browser    *rod.Browser
	contexts   map[string]*ManagedContext // key: "default" | "private" | "proxy:<url>"
	stop       chan struct{}
	done       chan struct{}

	// test-only injection: sleep before newPageInContext to simulate slow CDP.
	newPageDelay time.Duration
}

// ManagedContext is a Chrome BrowserContext with a set of named pages.
type ManagedContext struct {
	Mu    sync.Mutex
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
//
// CDP calls run OUTSIDE any lock to avoid blocking List/SessionCount callers.
func (p *ContextPool) GetOrCreatePage(session, mode, proxy, url string) (*ManagedPage, error) {
	key := contextKey(mode, proxy)

	// Phase 1: get or create context (CDP BrowserContext creation runs unlocked).
	mc, err := p.getOrCreateContextSafe(key, mode, proxy)
	if err != nil {
		return nil, err
	}

	// Phase 2: look up existing session under per-context lock.
	mc.Mu.Lock()
	if mp, ok := mc.Pages[session]; ok {
		mp.LastUsed = time.Now()
		if url != "" && url != "about:blank" && url != mp.URL {
			mp.URL = url
		}
		mc.Mu.Unlock()
		return mp, nil
	}
	session = deduplicateSession(mc, session)

	// Adopt-existing-tab fast path for default context (drop lock for CDP call).
	if key == "default" && len(mc.Pages) == 0 {
		mc.Mu.Unlock()
		if adopted, aerr := p.adoptExistingPage(mc); aerr == nil && adopted != nil {
			mp := &ManagedPage{Session: session, Page: adopted, URL: url, LastUsed: time.Now(), TTL: contextPoolDefaultTTL, Refs: NewRefMap()}
			mc.Mu.Lock()
			if existing, ok := mc.Pages[session]; ok {
				mc.Mu.Unlock()
				_ = adopted.Close()
				return existing, nil
			}
			mc.Pages[session] = mp
			mc.Mu.Unlock()
			return mp, nil
		}
		mc.Mu.Lock()
	}

	// Phase 3: reserve placeholder, release lock, do CDP.
	mc.Pages[session] = &ManagedPage{Session: session, LastUsed: time.Now(), TTL: contextPoolDefaultTTL, Refs: NewRefMap(), URL: url}
	mc.Mu.Unlock()

	if p.newPageDelay > 0 {
		time.Sleep(p.newPageDelay) // test injection — simulates slow CDP
	}
	page, err := p.newPageInContext(mc)
	if err != nil {
		mc.Mu.Lock()
		delete(mc.Pages, session)
		mc.Mu.Unlock()
		return nil, fmt.Errorf("context_pool: create tab in context %q: %w", key, err)
	}

	// Phase 4: patch placeholder with real page.
	mc.Mu.Lock()
	mp := mc.Pages[session]
	if mp == nil {
		// Reaped while we were in CDP — give up.
		mc.Mu.Unlock()
		_ = page.Close()
		return nil, fmt.Errorf("context_pool: session %q was reaped during creation", session)
	}
	mp.Page = page
	mc.Mu.Unlock()
	return mp, nil
}

// ClosePage closes a specific session's page. Disposes context if no pages remain.
// The default context is never disposed.
func (p *ContextPool) ClosePage(session string) error {
	var (
		page     *rod.Page
		mc       *ManagedContext
		emptyKey string
	)

	p.contextsMu.RLock()
	for key, c := range p.contexts {
		c.Mu.Lock()
		mp, ok := c.Pages[session]
		if !ok {
			c.Mu.Unlock()
			continue
		}
		page = mp.Page
		delete(c.Pages, session)
		if len(c.Pages) == 0 && key != "default" {
			emptyKey = key
		}
		c.Mu.Unlock()
		mc = c
		break
	}
	p.contextsMu.RUnlock()

	if mc == nil {
		return fmt.Errorf("context_pool: session %q not found", session)
	}

	// Close page unlocked — may take seconds.
	closePageWithTimeout(page)

	// Dispose empty non-default context.
	if emptyKey != "" {
		p.contextsMu.Lock()
		if c, ok := p.contexts[emptyKey]; ok {
			c.Mu.Lock()
			stillEmpty := len(c.Pages) == 0
			c.Mu.Unlock()
			if stillEmpty {
				p.disposeContext(c)
				delete(p.contexts, emptyKey)
			}
		}
		p.contextsMu.Unlock()
	}
	return nil
}

// SessionCount returns the total number of active named sessions across all contexts.
func (p *ContextPool) SessionCount() int {
	p.contextsMu.RLock()
	ctxs := make([]*ManagedContext, 0, len(p.contexts))
	for _, mc := range p.contexts {
		ctxs = append(ctxs, mc)
	}
	p.contextsMu.RUnlock()
	n := 0
	for _, mc := range ctxs {
		mc.Mu.Lock()
		n += len(mc.Pages)
		mc.Mu.Unlock()
	}
	return n
}

// List returns all contexts and their sessions.
func (p *ContextPool) List() []ContextInfo {
	p.contextsMu.RLock()
	ctxs := make([]*ManagedContext, 0, len(p.contexts))
	for _, mc := range p.contexts {
		ctxs = append(ctxs, mc)
	}
	p.contextsMu.RUnlock()

	result := make([]ContextInfo, 0, len(ctxs))
	for _, mc := range ctxs {
		mc.Mu.Lock()
		ci := ContextInfo{Mode: mc.Mode, Proxy: mc.Proxy, Sessions: make([]SessionInfo, 0, len(mc.Pages))}
		for _, mp := range mc.Pages {
			ci.Sessions = append(ci.Sessions, SessionInfo{
				Name:     mp.Session,
				URL:      mp.URL,
				LastUsed: formatAge(mp.LastUsed),
			})
		}
		mc.Mu.Unlock()
		result = append(result, ci)
	}
	return result
}

// Reap closes expired pages and disposes empty non-default contexts.
// Page closes run unlocked to avoid blocking callers.
func (p *ContextPool) Reap() {
	type victim struct {
		page *rod.Page
		key  string
		name string
	}
	var victims []victim

	p.contextsMu.RLock()
	for key, mc := range p.contexts {
		mc.Mu.Lock()
		for name, mp := range mc.Pages {
			if mp.TTL > 0 && time.Since(mp.LastUsed) > mp.TTL {
				victims = append(victims, victim{page: mp.Page, key: key, name: name})
				delete(mc.Pages, name)
			}
		}
		mc.Mu.Unlock()
	}
	p.contextsMu.RUnlock()

	// Close pages unlocked.
	for _, v := range victims {
		closePageWithTimeout(v.page)
	}

	// Dispose empty non-default contexts.
	p.contextsMu.Lock()
	for key, mc := range p.contexts {
		if key == "default" {
			continue
		}
		mc.Mu.Lock()
		empty := len(mc.Pages) == 0
		mc.Mu.Unlock()
		if empty {
			p.disposeContext(mc)
			delete(p.contexts, key)
		}
	}
	p.contextsMu.Unlock()
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
	p.contextsMu.Lock()
	p.browser = b
	p.contextsMu.Unlock()
}
