package browser

import (
	"sync"
	"testing"
	"time"
)

// TestSessionPool_ConcurrentCreateGet spawns 10 goroutines creating sessions
// and 10 goroutines getting them. Must not panic or race under -race.
func TestSessionPool_ConcurrentCreateGet(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	const workers = 10
	ids := make([]string, workers)

	// Pre-create sessions so getters have something to retrieve.
	for i := range ids {
		id, err := pool.Create("proxy")
		if err != nil {
			t.Fatalf("pre-create[%d]: %v", i, err)
		}
		ids[i] = id
	}

	var wg sync.WaitGroup
	wg.Add(workers * 2)

	// 10 creators
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, _ = pool.Create("proxy-concurrent")
		}()
	}

	// 10 getters hitting pre-created sessions
	for i := 0; i < workers; i++ {
		id := ids[i]
		go func() {
			defer wg.Done()
			_, _ = pool.Get(id)
		}()
	}

	wg.Wait()
}

// TestSessionPool_ConcurrentCreateDestroy creates and destroys the same session
// from different goroutines. Must not panic.
func TestSessionPool_ConcurrentCreateDestroy(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	const rounds = 50
	var wg sync.WaitGroup
	wg.Add(rounds * 2)

	for i := 0; i < rounds; i++ {
		id, err := pool.Create("proxy")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Destroyer
		go func(sid string) {
			defer wg.Done()
			pool.Destroy(sid)
		}(id)

		// Concurrent getter — may or may not find it
		go func(sid string) {
			defer wg.Done()
			_, _ = pool.Get(sid)
		}(id)
	}

	wg.Wait()
}

// TestSessionPool_GetRefreshesLastUsed verifies that calling Get refreshes
// LastUsed, which prevents premature expiry on actively-used sessions.
func TestSessionPool_GetRefreshesLastUsed(t *testing.T) {
	pool := NewSessionPool(200*time.Millisecond, 0)
	defer pool.Close()

	id, err := pool.Create("proxy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Sleep for half the TTL, then Get to refresh LastUsed.
	time.Sleep(120 * time.Millisecond)

	s, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get (first): %v", err)
	}
	before := s.LastUsed

	// Sleep another 120 ms — without refresh the session would have expired.
	time.Sleep(120 * time.Millisecond)

	s2, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get (second, after 240ms total): session should be alive due to LastUsed refresh: %v", err)
	}
	if !s2.LastUsed.After(before) {
		t.Error("Get: LastUsed must advance on each access")
	}
}

// TestSessionPool_ZeroTTL_NeverExpires verifies that a pool with TTL=0
// never expires sessions regardless of idle time.
func TestSessionPool_ZeroTTL_NeverExpires(t *testing.T) {
	pool := NewSessionPool(0, 0)
	defer pool.Close()

	id, err := pool.Create("proxy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manipulate LastUsed to simulate a very old session.
	pool.mu.Lock()
	pool.sessions[id].LastUsed = time.Now().Add(-24 * time.Hour)
	pool.mu.Unlock()

	// Explicit eviction pass — must NOT remove zero-TTL sessions.
	pool.evictExpired()

	if pool.Count() != 1 {
		t.Errorf("evictExpired: zero-TTL session must survive eviction, count = %d", pool.Count())
	}

	if _, err := pool.Get(id); err != nil {
		t.Errorf("Get: zero-TTL session must be retrievable after eviction pass: %v", err)
	}
}

// TestSessionPool_IsExpiredAfterGet verifies that Get refreshes LastUsed so
// isExpired returns false even when the session was created long ago.
func TestSessionPool_IsExpiredAfterGet(t *testing.T) {
	pool := NewSessionPool(100*time.Millisecond, 0)
	defer pool.Close()

	id, err := pool.Create("proxy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Back-date CreatedAt to simulate an old session, but keep LastUsed fresh
	// by calling Get.
	pool.mu.Lock()
	pool.sessions[id].CreatedAt = time.Now().Add(-1 * time.Hour)
	pool.mu.Unlock()

	// Get refreshes LastUsed.
	if _, err := pool.Get(id); err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Now check isExpired directly — LastUsed was just refreshed, so it must be false.
	pool.mu.Lock()
	s := pool.sessions[id]
	expired := s.isExpired()
	pool.mu.Unlock()

	if expired {
		t.Error("isExpired: must be false immediately after Get refreshes LastUsed")
	}
}

// TestSessionPool_DoubleDestroy verifies that destroying a session twice
// returns false on the second call and does not panic.
func TestSessionPool_DoubleDestroy(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	id, err := pool.Create("proxy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if ok := pool.Destroy(id); !ok {
		t.Error("first Destroy: expected true")
	}
	if ok := pool.Destroy(id); ok {
		t.Error("second Destroy: expected false for already-destroyed session")
	}
}

// TestSessionPool_GetAfterClose verifies that Get after Close returns an
// error and does not panic.
func TestSessionPool_GetAfterClose(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)

	id, err := pool.Create("proxy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	pool.Close()

	_, err = pool.Get(id)
	if err == nil {
		t.Error("Get after Close: expected error, got nil")
	}
}

// TestSessionPool_CreateAfterClose verifies that Create after Close returns
// an error and does not panic.
func TestSessionPool_CreateAfterClose(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	pool.Close()

	_, err := pool.Create("proxy")
	if err == nil {
		t.Error("Create after Close: expected error, got nil")
	}
}

// TestSessionPool_EvictExpiredNoOp verifies that evictExpired with no expired
// sessions leaves the count unchanged.
func TestSessionPool_EvictExpiredNoOp(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	for i := 0; i < 5; i++ {
		if _, err := pool.Create("proxy"); err != nil {
			t.Fatalf("Create[%d]: %v", i, err)
		}
	}

	beforeCount := pool.Count()
	pool.evictExpired()
	afterCount := pool.Count()

	if afterCount != beforeCount {
		t.Errorf("evictExpired (no expired): count changed from %d to %d", beforeCount, afterCount)
	}
}

// TestSessionPool_MaxSessionsEnforced verifies that Create returns an error
// when the pool is at its configured MaxConcurrent limit.
func TestSessionPool_MaxSessionsEnforced(t *testing.T) {
	const max = 3
	pool := NewSessionPool(time.Minute, max)
	defer pool.Close()

	for i := 0; i < max; i++ {
		if _, err := pool.Create("proxy"); err != nil {
			t.Fatalf("Create[%d] within limit: %v", i, err)
		}
	}

	if pool.Count() != max {
		t.Fatalf("Count: want %d, got %d", max, pool.Count())
	}

	// One more must fail.
	_, err := pool.Create("proxy-overflow")
	if err == nil {
		t.Errorf("Create beyond MaxConcurrent (%d): expected error, got nil", max)
	}

	// Count must still be max.
	if c := pool.Count(); c != max {
		t.Errorf("Count after overflow attempt: want %d, got %d", max, c)
	}
}
