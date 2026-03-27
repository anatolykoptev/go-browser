package browser

import (
	"testing"
	"time"
)

func TestSessionPool_CreateAndGet(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	id, err := pool.Create("http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if id == "" {
		t.Fatal("Create: expected non-empty ID")
	}

	s, ok := pool.Get(id)
	if !ok {
		t.Fatalf("Get(%q): expected session to exist", id)
	}
	if s.ID != id {
		t.Errorf("Get: session ID = %q, want %q", s.ID, id)
	}
	if s.Proxy != "http://proxy.example.com:8080" {
		t.Errorf("Get: session Proxy = %q, want proxy URL", s.Proxy)
	}
	if s.CreatedAt.IsZero() {
		t.Error("Get: CreatedAt is zero")
	}
}

func TestSessionPool_Destroy(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	id, err := pool.Create("proxy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if ok := pool.Destroy(id); !ok {
		t.Error("Destroy: expected true for existing session")
	}
	if ok := pool.Destroy(id); ok {
		t.Error("Destroy: expected false for already-destroyed session")
	}
	if _, ok := pool.Get(id); ok {
		t.Error("Get after Destroy: expected session to be gone")
	}
}

func TestSessionPool_TTLExpiry(t *testing.T) {
	// Use a very short TTL; override reaper interval by calling evictExpired directly.
	pool := NewSessionPool(50*time.Millisecond, 0)
	defer pool.Close()

	id, err := pool.Create("proxy")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait past the TTL.
	time.Sleep(120 * time.Millisecond)

	// Trigger eviction manually (reaper fires every 30s in production).
	pool.evictExpired()

	if _, ok := pool.Get(id); ok {
		t.Error("session should have been evicted after TTL expiry")
	}
}

func TestSessionPool_Count(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	if c := pool.Count(); c != 0 {
		t.Errorf("Count: want 0, got %d", c)
	}

	id1, _ := pool.Create("proxy1")
	if c := pool.Count(); c != 1 {
		t.Errorf("Count after 1 create: want 1, got %d", c)
	}

	_, _ = pool.Create("proxy2")
	if c := pool.Count(); c != 2 {
		t.Errorf("Count after 2 creates: want 2, got %d", c)
	}

	pool.Destroy(id1)
	if c := pool.Count(); c != 1 {
		t.Errorf("Count after destroy: want 1, got %d", c)
	}
}

func TestSessionPool_MaxConcurrent(t *testing.T) {
	pool := NewSessionPool(time.Minute, 2)
	defer pool.Close()

	if _, err := pool.Create("p1"); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	if _, err := pool.Create("p2"); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if _, err := pool.Create("p3"); err == nil {
		t.Error("Create 3 beyond max: expected error, got nil")
	}
}

func TestSessionPool_GetUpdatesLastUsed(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	id, _ := pool.Create("proxy")

	s1, _ := pool.Get(id)
	t1 := s1.LastUsed

	time.Sleep(5 * time.Millisecond)

	s2, _ := pool.Get(id)
	if !s2.LastUsed.After(t1) {
		t.Error("Get: LastUsed should be updated on each access")
	}
}
