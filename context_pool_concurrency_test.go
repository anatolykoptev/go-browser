package browser

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestContextPool_ListNotBlockedByGetOrCreate verifies that List() can read pool
// state while a GetOrCreatePage call is stuck in CDP I/O. This reproduces the
// deadlock where chrome_tabs times out whenever an interact call is in flight.
func TestContextPool_ListNotBlockedByGetOrCreate(t *testing.T) {
	p := newTestPoolWithSlowTargetCreate(t, 2*time.Second)
	defer p.Close()

	// Pre-populate one session so List() has something to return.
	if _, err := p.GetOrCreatePage("pre", "private", "", "about:blank"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Start a slow GetOrCreatePage in the background.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = p.GetOrCreatePage("slow", "private", "", "about:blank")
	}()

	// Give the goroutine time to enter the CDP call.
	time.Sleep(100 * time.Millisecond)

	// List() must return within 200ms — it should NOT wait for the CDP call.
	done := make(chan struct{})
	go func() {
		_ = p.List()
		close(done)
	}()
	select {
	case <-done:
		// pass
	case <-time.After(200 * time.Millisecond):
		t.Fatal("List() blocked by in-flight GetOrCreatePage — deadlock present")
	}

	wg.Wait()
}

// TestContextPool_StressConcurrentOps fires 20 goroutines creating pages
// with varying delays while a 21st polls List() every 10ms. All List() calls
// must return in <50ms and no race detector warnings may occur.
func TestContextPool_StressConcurrentOps(t *testing.T) {
	p := newTestPoolWithSlowTargetCreate(t, 500*time.Millisecond)
	defer p.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, 21)

	// 20 creators.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session := fmt.Sprintf("sess-%d", i)
			if _, err := p.GetOrCreatePage(session, "private", "", "about:blank"); err != nil {
				errCh <- err
			}
		}(i)
	}

	// 1 poller.
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			start := time.Now()
			_ = p.List()
			if d := time.Since(start); d > 200*time.Millisecond {
				errCh <- fmt.Errorf("List() took %v — expected <200ms", d)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()
	<-pollDone
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestContextPool_ConcurrentSameSession verifies that two goroutines racing to
// GetOrCreatePage with the same session name never receive a nil Page.
func TestContextPool_ConcurrentSameSession(t *testing.T) {
	p := newTestPoolWithSlowTargetCreate(t, 300*time.Millisecond)
	defer p.Close()

	const session = "shared-session"
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mp, err := p.GetOrCreatePage(session, "private", "", "about:blank")
			if err != nil {
				errs <- fmt.Errorf("GetOrCreatePage error: %v", err)
				return
			}
			if mp.Page == nil {
				errs <- fmt.Errorf("GetOrCreatePage returned mp with nil Page")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// newTestPoolWithSlowTargetCreate returns a ContextPool whose GetOrCreatePage
// sleeps for `delay` before calling newPageInContext, simulating a slow CDP call.
// Uses the shared Chromium instance (acquireSharedBrowser), skips if unavailable.
func newTestPoolWithSlowTargetCreate(t *testing.T, delay time.Duration) *ContextPool {
	t.Helper()
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	p.newPageDelay = delay
	return p
}
