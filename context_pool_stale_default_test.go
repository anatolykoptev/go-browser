package browser

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

// TestContextPool_StaleDefaultContext_Recovers reproduces the production
// availability bug: the pool discovers + caches the default BrowserContextID
// once and never re-discovers it. After Chrome disposes/recreates its default
// context externally, the cached handle goes stale and EVERY new default-context
// page creation fails with a CDP "Failed to find browser context with id"
// error, forever, until the process restarts.
//
// The flow_walk loop hits this path because resolveSessionParams maps
// ReusePage=true -> session="__reuse__" + mode="default" (interact.go:296),
// so the whole observe/act/warmup loop runs in the default context. Once the
// default context already has >=1 page, the adopt-existing-page fast path is
// skipped and new sessions go through newPageInContext with the cached ID.
//
// Production smoking gun (2026-06-25): pool cached 19ACB0A6... while the live
// default BrowserContext was 491730D1.... The non-empty stale ID is the trigger,
// which only occurs with a persistent profile / VNC Chrome whose default context
// has a non-empty BrowserContextID (headless fresh-launch reports it empty).
//
// Pre-fix: the second GetOrCreatePage (after external disposal) returns the
// stale-context CDP error and never recovers.
// Post-fix: the pool detects the stale-handle error, re-discovers/recreates the
// live default BrowserContextID, retries once, and succeeds.
func TestContextPool_StaleDefaultContext_Recovers(t *testing.T) {
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	before := StaleContextRecoveryStats()

	// 1. First default-context page discovers + caches the default context.
	if _, err := p.GetOrCreatePage("sess-warm", "default", "", "about:blank"); err != nil {
		t.Fatalf("seed default page: %v", err)
	}

	p.contextsMu.RLock()
	mc := p.contexts["default"]
	p.contextsMu.RUnlock()
	if mc == nil {
		t.Fatal("default context was not cached")
	}

	// 2. Force the production stale state: the cached default ID is a non-empty
	//    BrowserContextID that has since been disposed. Create a real context,
	//    dispose it, and stamp its (now-dead) ID onto mc — this yields the exact
	//    prod condition: mc.ID points at a disposed context.
	res, cerr := proto.TargetCreateBrowserContext{DisposeOnDetach: true}.Call(br)
	if cerr != nil {
		t.Fatalf("create scratch ctx: %v", cerr)
	}
	staleID := res.BrowserContextID
	_ = proto.TargetDisposeBrowserContext{BrowserContextID: staleID}.Call(br)

	// Keep mc.Pages NON-EMPTY so the adopt-existing-page fast path is skipped and
	// the next session is forced through newPageInContext with the stale ID —
	// exactly the production path for the __reuse__ loop after warmup.
	mc.Mu.Lock()
	mc.ID = staleID
	mc.Pages = map[string]*ManagedPage{"sess-warm": {Session: "sess-warm"}}
	mc.Mu.Unlock()

	// 3. New default-context session. Pre-fix this fails because the pool
	//    blindly reuses the stale ID in newPageInContext.
	mp2, err := p.GetOrCreatePage("__reuse__", "default", "", "about:blank")
	if err != nil {
		if strings.Contains(err.Error(), "Failed to find browser context") {
			t.Fatalf("stale-default-context latch NOT recovered (production bug reproduced): %v", err)
		}
		t.Fatalf("unexpected error creating default page after disposal: %v", err)
	}
	if mp2 == nil || mp2.Page == nil {
		t.Fatal("recovered default page is nil")
	}

	// 4. The pool must have re-discovered/recreated a live default context ID,
	//    not kept latching the disposed one.
	p.contextsMu.RLock()
	mc2 := p.contexts["default"]
	p.contextsMu.RUnlock()
	if mc2 != nil && mc2.ID == staleID {
		t.Errorf("pool still latches the disposed default context ID %q after recovery", staleID)
	}

	// 5. Observability must have fired: detected + recovered each incremented.
	after := StaleContextRecoveryStats()
	if after[StaleCtxOutcomeDetected] <= before[StaleCtxOutcomeDetected] {
		t.Error("chrome_stale_context_recovery_total{outcome=detected} did not increment")
	}
	if after[StaleCtxOutcomeRecovered] <= before[StaleCtxOutcomeRecovered] {
		t.Error("chrome_stale_context_recovery_total{outcome=recovered} did not increment")
	}
}

// TestContextPool_PrivateContext_NoStaleRecovery proves the recovery path is a
// strict no-op for non-default callers (private/proxy) — the change must not
// alter behaviour for ox-browser or any consumer that creates its own context.
// A genuinely failing private-context creation must surface the error directly,
// without a re-discovery retry and without touching the recovery counters.
func TestContextPool_PrivateContext_NoStaleRecovery(t *testing.T) {
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	before := StaleContextRecoveryStats()

	// Build a private context, then dispose its BrowserContext out from under the
	// pool so the next create fails with the same CDP error class.
	mc := &ManagedContext{Mode: "private", Pages: map[string]*ManagedPage{}}
	res, err := proto.TargetCreateBrowserContext{DisposeOnDetach: true}.Call(br)
	if err != nil {
		t.Fatalf("create private ctx: %v", err)
	}
	mc.ID = res.BrowserContextID
	_ = proto.TargetDisposeBrowserContext{BrowserContextID: mc.ID}.Call(br)

	// Private context must NOT attempt recovery: the error propagates, counters
	// stay flat (key != "default").
	_, perr := p.createPageWithStaleRecovery(mc, "private")
	if perr == nil {
		t.Fatal("expected private-context create to fail on disposed context")
	}
	if !strings.Contains(perr.Error(), "Failed to find browser context") {
		t.Fatalf("unexpected private-context error: %v", perr)
	}

	after := StaleContextRecoveryStats()
	if after[StaleCtxOutcomeDetected] != before[StaleCtxOutcomeDetected] ||
		after[StaleCtxOutcomeRecovered] != before[StaleCtxOutcomeRecovered] ||
		after[StaleCtxOutcomeFailed] != before[StaleCtxOutcomeFailed] {
		t.Error("private-context failure must not touch chrome_stale_context_recovery_total counters")
	}
}

// TestContextPool_ConcurrentDefaultStaleRecovery_NoRace exercises the recovery
// path under concurrent default-context creates with the race detector. Two
// goroutines create distinct default sessions while the cached default ID is
// stale, so both may reach createPageWithStaleRecovery and race the mc.ID
// read (newPageInContext) against the mc.ID write (rediscoverDefaultContext).
// With -race this fails if mc.ID is not consistently mc.Mu-guarded.
func TestContextPool_ConcurrentDefaultStaleRecovery_NoRace(t *testing.T) {
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	if _, err := p.GetOrCreatePage("seed", "default", "", "about:blank"); err != nil {
		t.Fatalf("seed default page: %v", err)
	}
	p.contextsMu.RLock()
	mc := p.contexts["default"]
	p.contextsMu.RUnlock()
	if mc == nil {
		t.Fatal("default context not cached")
	}

	// Stamp a disposed (stale) non-empty ID and keep Pages non-empty so the
	// adopt fast path is skipped and both creates go through newPageInContext.
	res, err := proto.TargetCreateBrowserContext{DisposeOnDetach: true}.Call(br)
	if err != nil {
		t.Fatalf("scratch ctx: %v", err)
	}
	_ = proto.TargetDisposeBrowserContext{BrowserContextID: res.BrowserContextID}.Call(br)
	mc.Mu.Lock()
	mc.ID = res.BrowserContextID
	mc.Pages = map[string]*ManagedPage{"seed": {Session: "seed"}}
	mc.Mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Errors are acceptable here (one goroutine may win recovery and
			// invalidate the other's stale view); the assertion is "no race".
			_, _ = p.GetOrCreatePage(fmt.Sprintf("c-%d", i), "default", "", "about:blank")
		}(i)
	}
	wg.Wait()
}
