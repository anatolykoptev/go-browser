package browser_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-browser"
)

func TestPool_LimitsConcurrency(t *testing.T) {
	pool := browser.NewPool(2)
	defer pool.Close()

	var running atomic.Int32
	var maxSeen atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := pool.Acquire(context.Background())
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			defer release()

			cur := running.Add(1)
			for {
				old := maxSeen.Load()
				if cur <= old || maxSeen.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			running.Add(-1)
		}()
	}
	wg.Wait()

	if maxSeen.Load() > 2 {
		t.Errorf("max concurrent = %d, want <= 2", maxSeen.Load())
	}
}

func TestPool_RespectsContextCancel(t *testing.T) {
	pool := browser.NewPool(1)
	defer pool.Close()

	release, _ := pool.Acquire(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := pool.Acquire(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	release()
}

func TestPool_MinSize(t *testing.T) {
	pool := browser.NewPool(0) // should clamp to 1
	defer pool.Close()

	release, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// Pool size 1 — second acquire should block.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = pool.Acquire(ctx)
	if err == nil {
		t.Fatal("expected timeout on pool of size 1")
	}
	release()
}

func TestPool_CloseUnblocksWaiters(t *testing.T) {
	pool := browser.NewPool(1)

	// Fill the pool.
	release, _ := pool.Acquire(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := pool.Acquire(context.Background())
		done <- err
	}()

	// Give goroutine time to block on Acquire.
	time.Sleep(10 * time.Millisecond)

	pool.Close()
	release()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("waiter not unblocked after Close")
	}
}

func TestPool_DoubleCloseNoPanic(t *testing.T) {
	pool := browser.NewPool(2)
	pool.Close()
	pool.Close() // should not panic
}
