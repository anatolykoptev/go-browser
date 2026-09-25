package browser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
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
	pageCreationTimeout       = 30 * time.Second // #41: max time to create a page via CDP
	pageLivenessTimeout       = 2 * time.Second  // #79: bounded Info() probe on pooled-page checkout
)

// ContextPool manages named browser sessions grouped by context (default/private/proxy).
// Each context maps to a Chrome BrowserContext; each session is a named tab within it.
//
// Locking discipline:
//   - contextsMu (RWMutex) guards the contexts map itself.
//   - ManagedContext.Mu (Mutex) guards that context's Pages map.
//   - browser is stored as atomic.Pointer[rod.Browser] for lock-free, nil-safe reads.
//     UpdateBrowser() atomically swaps the pointer; all readers use Load() without any lock.
//   - CDP I/O (TargetCreateTarget, Page.Close, etc.) runs OUTSIDE any lock.
//
// Lock ordering (must be acquired in this order if both are held):
//  1. contextsMu (global pool lock)
//  2. ManagedContext.Mu (per-context lock)
//
// Never acquire in reverse order — deadlock risk. Never hold either lock across a CDP call.
type ContextPool struct {
	contextsMu sync.RWMutex
	browser    atomic.Pointer[rod.Browser]
	contexts   map[string]*ManagedContext // key: "default" | "private" | "proxy:<url>"
	stop       chan struct{}
	done       chan struct{}

	// generation is incremented on each UpdateBrowser (reconnect). ManagedPage
	// records the generation at creation time; IsValid() checks it matches.
	// This detects stale page references after reconnect (Playwright _browserClosed
	// pattern + Vercel agent-browser generation counter).
	generation atomic.Uint64

	// genCtx is the per-generation context: cancelled once by UpdateBrowser,
	// which fails every in-flight CDP call on pages of the previous generation
	// at once (Playwright fail-all-pending-callbacks-on-disconnect). Each
	// ManagedPage.lifeCtx is a child of genCtx.
	genMu     sync.Mutex
	genCtx    context.Context
	genCancel context.CancelFunc

	// stealthProfile, when non-nil, is automatically applied to every new page
	// created via GetOrCreatePage (puppeteer-extra onPageCreated pattern).
	// This ensures stealth is applied on ALL page creation paths, not just
	// RunInteract. Set via SetStealthProfile.
	stealthProfile *StealthProfile

	// test-only injection: sleep before newPageInContext to simulate slow CDP.
	newPageDelay time.Duration
}

// SetStealthProfile sets the profile that will be automatically applied to
// every new page created via GetOrCreatePage. This centralizes stealth
// application so pages created outside RunInteract (e.g., via the pool
// directly) also get stealth. puppeteer-extra onPageCreated pattern.
func (p *ContextPool) SetStealthProfile(profile *StealthProfile) {
	p.contextsMu.Lock()
	p.stealthProfile = profile
	p.contextsMu.Unlock()
}

// getBrowser returns the current browser atomically. Returns nil if not connected
// (e.g., during reconnect). Callers must check for nil before use.
func (p *ContextPool) getBrowser() *rod.Browser {
	return p.browser.Load()
}

// getStealthProfile returns the pool's stealth profile under a read lock.
func (p *ContextPool) getStealthProfile() *StealthProfile {
	p.contextsMu.RLock()
	defer p.contextsMu.RUnlock()
	return p.stealthProfile
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
// ready is closed when Page is fully initialised; callers who find a
// placeholder (Page==nil) must wait on ready before using the page.
// mu protects LastUsed and URL after the page is ready.
type ManagedPage struct {
	mu           sync.Mutex
	Session      string
	Mode         string // resolved context mode: "default", "private", or "proxy"
	Page         *rod.Page
	ready        chan struct{} // closed when Page != nil (or creation failed)
	readyOnce    sync.Once     // ensures ready is closed exactly once
	readyErr     error         // non-nil if page creation failed
	URL          string
	LastUsed     time.Time
	TTL          time.Duration // 0 = never expires
	Refs         *RefMap
	LogCollector *LogCollector
	DetachedAt   time.Time // zero = attached (agent-controllable)
	generation   uint64    // pool generation at creation; mismatch = stale after reconnect
	// lifeCtx dies when the tab dies (targetDestroyed), the page is closed or
	// reaped, or the browser reconnects. Page itself is bound to it at
	// publication, so every rod call on the pooled page is cancel-on-death
	// (Playwright _closedOrCrashedScope equivalent). RunInteract rebinds a
	// merged request+lifecycle ctx per call.
	lifeCtx    context.Context
	lifeCancel context.CancelFunc
}

// signalReady closes the ready channel exactly once. Safe to call multiple times.
func (mp *ManagedPage) signalReady() {
	mp.readyOnce.Do(func() { close(mp.ready) })
}

// IsValid reports whether this page belongs to the current browser generation.
// After reconnect, all pages from the previous generation are invalid (their
// rod.Page references point to a closed CDP connection).
func (mp *ManagedPage) IsValid(pool *ContextPool) bool {
	return mp != nil && mp.generation == pool.generation.Load()
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
		contexts: make(map[string]*ManagedContext),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	p.genCtx, p.genCancel = context.WithCancel(context.Background())
	p.browser.Store(browser)
	go p.reaper()
	p.watchTargetDestroyed()
	return p
}

// newPageLifecycle derives a per-page cancel context under the current pool
// generation. Firing the returned cancel is the cancel-on-death signal: every
// in-flight rod call on the page returns ctx.Err() immediately instead of
// waiting for a CDP response that will never arrive.
func (p *ContextPool) newPageLifecycle() (context.Context, context.CancelFunc) {
	p.genMu.Lock()
	defer p.genMu.Unlock()
	return context.WithCancel(p.genCtx)
}

// cancelPageLife fires the page's lifecycle cancel, if set.
func cancelPageLife(mp *ManagedPage) {
	if mp != nil && mp.lifeCancel != nil {
		mp.lifeCancel()
	}
}

// pageAlive probes a pooled page with a short deadline (validate-on-checkout,
// like database/sql Ping). A dead tab — target destroyed without a delivered
// event, or a silently detached session — fails fast instead of handing the
// caller an unresponsive *rod.Page.
func (p *ContextPool) pageAlive(mp *ManagedPage) bool {
	if mp == nil || mp.Page == nil || mp.lifeCtx == nil {
		return false
	}
	if mp.lifeCtx.Err() != nil {
		return false // lifecycle already dead — no CDP call needed
	}
	b := p.getBrowser()
	if b == nil {
		return false
	}
	// NOTE: Page.Info() ignores the page's ctx — it delegates to
	// browser.pageInfo() which uses the browser's ctx. The probe must bind a
	// browser clone, not the page.
	probe, cancel := context.WithTimeout(mp.lifeCtx, pageLivenessTimeout)
	defer cancel()
	_, err := (proto.TargetGetTargetInfo{TargetID: mp.Page.TargetID}).Call(b.Context(probe))
	if err != nil {
		slog.Warn("pageAlive: probe failed", "session", mp.Session, "target", mp.Page.TargetID, "err", err)
	}
	return err == nil
}

// GetOrCreatePage returns the existing page for session, or creates a new tab in the
// appropriate context. If session is empty an ephemeral name is generated.
//
// Rule 1 (#74): a named session (session != "") with an empty mode defaults to
// the persistent context ("default"). A named session is a request for
// continuity; giving it an ephemeral jar contradicts its own purpose. An
// anonymous call (empty session) must opt into ephemeral explicitly.
//
// Rule 2 (#74): an unrecognised mode (e.g. "defualt") is a typed error
// (ErrInvalidMode) — the old contextKey default-arm silently absorbed typos
// into an incognito context, making an authenticated session indistinguishable
// from an expired one.
//
// Rule 3 (#74): the resolved mode is stored on the returned ManagedPage.Mode
// so a consumer can assert which context was actually used. ManagedPage.Mode is
// an opt-in observability field — it is written on every page-returning path but
// is NOT read by any internal code in this package; it exists for external
// consumers (go-wowa, callers of GetOrCreatePage). The resolved mode is also
// logged at context creation (getOrCreateContextSafe) so the resolved context is
// surfaced in logs without a reader of this field.
//
// CDP calls run OUTSIDE any lock to avoid blocking List/SessionCount callers.
func (p *ContextPool) GetOrCreatePage(session, mode, proxy, url string) (*ManagedPage, error) {
	// Rule 1: named session + empty mode → persistent context. With a proxy
	// the persistent context is the proxy context (egress through that proxy),
	// matching resolveSessionParams in interact.go. Hardcoding "default" here
	// would drop the proxy: contextKey("default", proxy) yields "default", and
	// getOrCreateContextSafe's default branch never sets proxyServer — the
	// caller would get an unproxied context (proxy bypass / datacenter-IP leak).
	if session != "" && mode == "" {
		if proxy != "" {
			mode = modeProxy
		} else {
			mode = modeDefault
		}
	}
	key, err := contextKey(mode, proxy)
	if err != nil {
		return nil, err
	}

	// Phase 1: get or create context (CDP BrowserContext creation runs unlocked).
	mc, err := p.getOrCreateContextSafe(key, mode, proxy)
	if err != nil {
		return nil, err
	}

	// Phase 2: look up existing session under per-context lock.
	mc.Mu.Lock()
	if mp, ok := mc.Pages[session]; ok {
		mc.Mu.Unlock()
		// Wait for in-flight creation to finish (Page may be nil placeholder).
		<-mp.ready
		if mp.readyErr != nil {
			return nil, mp.readyErr
		}
		// #79: validate-on-checkout — a page whose target died without a
		// delivered TargetDestroyed event is a corpse; hand out a fresh tab
		// instead of letting the caller discover it via a hung CDP call.
		if !p.pageAlive(mp) {
			mc.Mu.Lock()
			if cur, ok := mc.Pages[session]; ok && cur == mp {
				delete(mc.Pages, session)
			}
			cancelPageLife(mp)
			// Do NOT close the underlying tab: if it is still alive the
			// adopt path below reuses it for the replacement — closing here
			// races TargetDestroyed and kills the fresh page.
			// Lock is held: fall through to deduplicateSession/create below.
		} else {
			mp.mu.Lock()
			mp.LastUsed = time.Now()
			if url != "" && url != "about:blank" && url != mp.URL {
				mp.URL = url
			}
			mp.mu.Unlock()
			return mp, nil
		}
	}
	session = deduplicateSession(mc, session)

	// Adopt-existing-tab fast path for default context (drop lock for CDP call).
	if key == "default" && len(mc.Pages) == 0 {
		mc.Mu.Unlock()
		if adopted, aerr := p.adoptExistingPage(mc); aerr == nil && adopted != nil {
			curGen := p.generation.Load()
			lc := NewLogCollector()
			if p.getStealthProfile() != nil {
				lc = NewStealthLogCollector()
			}
			lifeCtx, lifeCancel := p.newPageLifecycle()
			mp := &ManagedPage{Session: session, Mode: mode, Page: adopted.Context(lifeCtx), ready: make(chan struct{}), URL: url, LastUsed: time.Now(), TTL: contextPoolDefaultTTL, Refs: NewRefMap(), LogCollector: lc, generation: curGen, lifeCtx: lifeCtx, lifeCancel: lifeCancel}
			mp.signalReady() // close ready immediately — adopted page is already live
			mc.Mu.Lock()
			if existing, ok := mc.Pages[session]; ok {
				mc.Mu.Unlock()
				cancelPageLife(mp)
				_ = adopted.Close()
				<-existing.ready
				if existing.readyErr != nil {
					return nil, existing.readyErr
				}
				return existing, nil
			}
			mc.Pages[session] = mp
			mc.Mu.Unlock()
			// #28: Apply stealth to adopted pages too.
			if profile := p.getStealthProfile(); profile != nil {
				if err := applyStealthToExistingPage(adopted, profile); err != nil {
					slog.Warn("context_pool: auto-stealth on adopted page failed", "session", session, "err", err)
				}
			}
			// Adopted page is already live — subscribe LogCollector immediately.
			mp.LogCollector.SubscribeCDP(mp.Page)
			return mp, nil
		}
		mc.Mu.Lock()
	}

	// Phase 3: reserve placeholder, release lock, do CDP.
	curGen := p.generation.Load()
	placeholderLC := NewLogCollector()
	if p.getStealthProfile() != nil {
		placeholderLC = NewStealthLogCollector()
	}
	lifeCtx, lifeCancel := p.newPageLifecycle()
	placeholder := &ManagedPage{
		Session:      session,
		Mode:         mode,
		ready:        make(chan struct{}),
		LastUsed:     time.Now(),
		TTL:          contextPoolDefaultTTL,
		Refs:         NewRefMap(),
		LogCollector: placeholderLC,
		URL:          url,
		generation:   curGen,
		lifeCtx:      lifeCtx,
		lifeCancel:   lifeCancel,
	}
	mc.Pages[session] = placeholder
	mc.Mu.Unlock()

	if p.newPageDelay > 0 {
		time.Sleep(p.newPageDelay) // test injection — simulates slow CDP
	}
	// #41: Add a timeout to page creation so a hung CDP call doesn't leave a
	// placeholder with Page==nil indefinitely. The placeholder is cleaned up
	// and waiters get an explicit error.
	pageCh := make(chan *rod.Page, 1)
	errCh := make(chan error, 1)
	go func() {
		pg, err := p.createPageWithStaleRecovery(mc, key)
		pageCh <- pg
		errCh <- err
	}()
	var page *rod.Page
	var cdpErr error
	select {
	case page = <-pageCh:
		cdpErr = <-errCh
	case <-time.After(pageCreationTimeout):
		cdpErr = fmt.Errorf("context_pool: create tab in context %q timed out after %s", key, pageCreationTimeout)
	}

	// Phase 4: patch placeholder and signal waiters regardless of outcome.
	mc.Mu.Lock()
	mp := mc.Pages[session]
	if mp == nil {
		// Reaped while we were in CDP, or invalidated by reconnect (generation changed).
		mc.Mu.Unlock()
		if page != nil {
			_ = page.Close()
		}
		cancelPageLife(placeholder)
		placeholder.signalReady() // unblock any waiters with nil page
		placeholder.readyErr = fmt.Errorf("context_pool: session %q was reaped during creation", session)
		return nil, placeholder.readyErr
	}
	if cdpErr != nil {
		delete(mc.Pages, session)
		mc.Mu.Unlock()
		cancelPageLife(placeholder)
		placeholder.readyErr = fmt.Errorf("context_pool: create tab in context %q: %w", key, cdpErr)
		placeholder.signalReady()
		return nil, placeholder.readyErr
	}
	// If reconnect happened during CDP, the page belongs to the old browser — discard it.
	if mp.generation != p.generation.Load() {
		delete(mc.Pages, session)
		mc.Mu.Unlock()
		cancelPageLife(placeholder)
		_ = page.Close()
		placeholder.readyErr = fmt.Errorf("context_pool: session %q invalidated by reconnect during creation", session)
		placeholder.signalReady()
		return nil, placeholder.readyErr
	}
	mp.Page = page.Context(mp.lifeCtx)
	mc.Mu.Unlock()
	// #28: Apply stealth automatically if a profile is set on the pool.
	// This ensures pages created via the pool (not just via RunInteract) get
	// stealth. puppeteer-extra onPageCreated pattern.
	if profile := p.getStealthProfile(); profile != nil {
		if err := applyStealthToExistingPage(page, profile); err != nil {
			// Log the error but don't fail — the page is usable without stealth,
			// just less protected. Caller can check via NoStealth flag.
			slog.Warn("context_pool: auto-stealth application failed", "session", session, "err", err)
		}
	}
	// Wire LogCollector to the real page. SubscribeCDP starts a listener goroutine
	// that runs until the page is closed.
	if mp.LogCollector != nil {
		mp.LogCollector.SubscribeCDP(mp.Page)
	}
	placeholder.signalReady()
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

	// Phase 1: find the session under RLock — don't block on ready channel
	// while holding contextsMu (gostall: starvation).
	var readyCh chan struct{}
	p.contextsMu.RLock()
	for key, c := range p.contexts {
		c.Mu.Lock()
		mp, ok := c.Pages[session]
		if !ok {
			c.Mu.Unlock()
			continue
		}
		readyCh = mp.ready
		mc = c
		emptyKey = key
		c.Mu.Unlock()
		break
	}
	p.contextsMu.RUnlock()

	if mc == nil {
		return fmt.Errorf("context_pool: session %q not found", session)
	}

	// Phase 2: wait for placeholder ready outside any pool lock.
	if readyCh != nil {
		<-readyCh
	}

	// Phase 3: re-acquire locks to delete and extract page.
	mc.Mu.Lock()
	mp, ok := mc.Pages[session]
	if !ok {
		// Page was reaped between phase 1 and 3 — nothing to close.
		mc.Mu.Unlock()
		return nil
	}
	page = mp.Page
	cancelPageLife(mp)
	delete(mc.Pages, session)
	if len(mc.Pages) != 0 || emptyKey == "default" {
		emptyKey = ""
	}
	mc.Mu.Unlock()

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
			mp.mu.Lock()
			si := SessionInfo{Name: mp.Session, URL: mp.URL, LastUsed: formatAge(mp.LastUsed)}
			mp.mu.Unlock()
			ci.Sessions = append(ci.Sessions, si)
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
		page   *rod.Page
		cancel context.CancelFunc
		key    string
		name   string
	}
	var victims []victim

	curGen := p.generation.Load()
	p.contextsMu.RLock()
	for key, mc := range p.contexts {
		mc.Mu.Lock()
		for name, mp := range mc.Pages {
			// Skip detached sessions - they are human-controlled, don't evict
			if !mp.DetachedAt.IsZero() {
				continue
			}
			// #60/#23: Reap stale pages after reconnect — generation mismatch
			// means the page's rod.Page points to a dead CDP connection.
			// Even detached pages are reaped if stale (their browser is gone).
			if mp.generation != curGen && mp.Page != nil {
				victims = append(victims, victim{page: mp.Page, cancel: mp.lifeCancel, key: key, name: name})
				delete(mc.Pages, name)
				continue
			}
			if mp.TTL > 0 && time.Since(mp.LastUsed) > mp.TTL && mp.Page != nil {
				victims = append(victims, victim{page: mp.Page, cancel: mp.lifeCancel, key: key, name: name})
				delete(mc.Pages, name)
			}
		}
		mc.Mu.Unlock()
	}
	p.contextsMu.RUnlock()

	// Close pages unlocked.
	for _, v := range victims {
		if v.cancel != nil {
			v.cancel()
		}
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

// UpdateBrowser replaces the browser reference atomically (after reconnect),
// increments the generation counter, and invalidates all existing pages.
// All readers using getBrowser() / p.browser.Load() will see the new browser
// immediately without any lock contention. Existing ManagedPage objects from
// the previous generation are cleared — their rod.Page references point to the
// closed CDP connection and must not be reused (Playwright _browserClosed
// cascading invalidation pattern).
func (p *ContextPool) UpdateBrowser(b *rod.Browser) {
	// Rotate under genMu so newPageLifecycle never hands out a lifecycle ctx
	// that is about to die: a creator either gets the old gen+old ctx (page
	// discarded by the phase-4 generation check) or the new gen+new ctx.
	p.genMu.Lock()
	if p.genCancel != nil {
		// Fail all in-flight calls on previous-generation pages at once:
		// their lifeCtx derives from the generation ctx cancelled here.
		p.genCancel()
	}
	p.genCtx, p.genCancel = context.WithCancel(context.Background())
	p.generation.Add(1)
	p.genMu.Unlock()
	p.browser.Store(b)

	// Invalidate all existing pages — they belong to the old browser generation.
	// Their rod.Page references are dead (CDP connection closed). Callers that
	// hold a ManagedPage reference can check IsValid() for an explicit error;
	// the pool's map is cleared so new GetOrCreatePage calls create fresh pages.
	p.contextsMu.Lock()
	for _, mc := range p.contexts {
		mc.Mu.Lock()
		mc.Pages = make(map[string]*ManagedPage)
		mc.Mu.Unlock()
	}
	p.contextsMu.Unlock()

	// The destruction watcher was subscribed on the old CDP connection and
	// died with it — resubscribe on the new browser so cancel-on-death keeps
	// working across reconnects.
	p.watchTargetDestroyed()
}

// FindManagedPage finds a managed page by session name across all contexts.
// Returns the ManagedPage and its containing ManagedContext.
func (p *ContextPool) FindManagedPage(sessionID string) (*ManagedPage, error) {
	p.contextsMu.RLock()
	defer p.contextsMu.RUnlock()

	for _, mc := range p.contexts {
		mc.Mu.Lock()
		mp, exists := mc.Pages[sessionID]
		mc.Mu.Unlock()
		if exists {
			return mp, nil
		}
	}

	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// DetachSession marks a session as human-controlled.
// Agent calls on this session will return an error and the reaper will skip it.
func (p *ContextPool) DetachSession(session, mode string) error {
	mp, err := p.FindManagedPage(session)
	if err != nil {
		return fmt.Errorf("session %q not found", session)
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.DetachedAt = time.Now()
	return nil
}

// AttachSession returns control of a session to the agent.
// The session becomes controllable again and will be subject to normal reaping.
func (p *ContextPool) AttachSession(session, mode string) error {
	mp, err := p.FindManagedPage(session)
	if err != nil {
		return fmt.Errorf("session %q not found", session)
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.DetachedAt = time.Time{}
	return nil
}
