package browser

import (
	"context"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// TestCancelOnDeath_InFlightEvalDies verifies cancel-on-death: an in-flight rod
// call on a pooled page returns promptly once the page's lifecycle ctx is
// cancelled, instead of hanging on a CDP response that never arrives.
//
// Requires a live Chrome — skipped under -short.
func TestCancelOnDeath_InFlightEvalDies(t *testing.T) {
	br := acquireSharedBrowser(t)
	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	mp, err := pool.GetOrCreatePage("test-cod-eval", "private", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage: %v", err)
	}

	// A JS promise that never resolves — Eval blocks until the ctx dies.
	done := make(chan error, 1)
	go func() {
		_, e := mp.Page.Eval(`() => new Promise(() => {})`)
		done <- e
	}()
	// Let the Eval get in-flight before killing the page.
	time.Sleep(300 * time.Millisecond)

	cancelPageLife(mp)

	select {
	case e := <-done:
		if e == nil {
			t.Fatal("expected error from killed Eval, got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("in-flight Eval did not die on lifecycle cancel")
	}
}

// TestUpdateBrowser_FailsInFlightCalls verifies the fail-all-pending behaviour:
// UpdateBrowser (reconnect) cancels the generation ctx, so every in-flight call
// on a previous-generation page unblocks immediately.
func TestUpdateBrowser_FailsInFlightCalls(t *testing.T) {
	br := acquireSharedBrowser(t)
	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	mp, err := pool.GetOrCreatePage("test-cod-reconn", "private", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, e := mp.Page.Eval(`() => new Promise(() => {})`)
		done <- e
	}()
	time.Sleep(300 * time.Millisecond)

	pool.UpdateBrowser(br)

	select {
	case e := <-done:
		if e == nil {
			t.Fatal("expected error from killed Eval, got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("in-flight Eval did not die on UpdateBrowser generation cancel")
	}
}

// TestWatchTargetDestroyed_CancelsLife verifies the watcher fires the page's
// lifecycle cancel when its target is destroyed at the browser level.
func TestWatchTargetDestroyed_CancelsLife(t *testing.T) {
	br := acquireSharedBrowser(t)
	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	mp, err := pool.GetOrCreatePage("test-cod-destroyed", "private", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage: %v", err)
	}

	life := mp.lifeCtx
	if life == nil {
		t.Fatal("page has no lifecycle ctx")
	}

	// Destroy the target directly at the browser level (same as Chrome killing
	// the tab) — the watcher must observe the event and cancel lifeCtx.
	if _, err := (proto.TargetCloseTarget{TargetID: mp.Page.TargetID}).Call(br); err != nil {
		t.Fatalf("TargetCloseTarget: %v", err)
	}

	select {
	case <-life.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("lifecycle ctx not cancelled after targetDestroyed")
	}

	// The page must also be gone from the pool.
	if _, err := pool.FindManagedPage("test-cod-destroyed"); err == nil {
		t.Fatal("destroyed page still in pool")
	}
}

// TestGetOrCreatePage_EvictsDeadPage verifies validate-on-checkout: a pooled
// page whose lifecycle is already dead is evicted and replaced with a fresh
// tab instead of being handed to the caller.
func TestGetOrCreatePage_EvictsDeadPage(t *testing.T) {
	br := acquireSharedBrowser(t)
	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	mp, err := pool.GetOrCreatePage("test-cod-evict", "private", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-cod-evict") })

	// Simulate a silently dead tab: lifecycle cancelled without a destruction
	// event (e.g. detached session, crashed renderer that ate the event).
	cancelPageLife(mp)

	mp2, err := pool.GetOrCreatePage("test-cod-evict", "private", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage after dead page: %v", err)
	}
	if mp2 == mp {
		t.Fatal("dead page was handed out again instead of being replaced")
	}
	if !pool.pageAlive(mp2) {
		t.Fatal("replacement page is not alive")
	}
}

// TestPageAlive_LiveAndDead covers the probe itself.
func TestPageAlive_LiveAndDead(t *testing.T) {
	br := acquireSharedBrowser(t)
	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	mp, err := pool.GetOrCreatePage("test-cod-alive", "private", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-cod-alive") })

	if !pool.pageAlive(mp) {
		t.Fatal("live page reported dead")
	}
	cancelPageLife(mp)
	if pool.pageAlive(mp) {
		t.Fatal("dead page reported alive")
	}
}

// TestRunInteract_DeadPageHeals verifies the full path: a request on a session
// whose tab died silently does not hang — validate-on-checkout evicts the
// corpse, a fresh tab is created, and the action completes.
func TestRunInteract_DeadPageHeals(t *testing.T) {
	br := acquireSharedBrowser(t)
	pool := NewContextPool(br)
	t.Cleanup(pool.Close)
	chrome := &ChromeManager{pool: pool, browser: br}

	sid := "test-cod-interact"
	mp, err := pool.GetOrCreatePage(sid, "private", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage(sid) })
	cancelPageLife(mp)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	resp := RunInteract(ctx, chrome, InteractRequest{
		Session:   sid,
		Mode:      "private",
		NoStealth: true,
		Actions:   []Action{{Type: "evaluate", Script: "1+1"}},
	})
	if resp.Status != "ok" {
		t.Fatalf("expected healed interact, got %q: %s", resp.Status, resp.Error)
	}
}
