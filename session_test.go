package browser

import (
	"strings"
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

	s, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get(%q): unexpected error: %v", id, err)
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
	if _, err := pool.Get(id); err == nil {
		t.Error("Get after Destroy: expected error, got nil")
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

	if _, err := pool.Get(id); err == nil {
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

	s1, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	t1 := s1.LastUsed

	time.Sleep(5 * time.Millisecond)

	s2, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !s2.LastUsed.After(t1) {
		t.Error("Get: LastUsed should be updated on each access")
	}
}

// --- TTL validation tests ---

func TestSession_IsExpired_FreshSession(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	id, _ := pool.Create("proxy")

	pool.mu.Lock()
	s := pool.sessions[id]
	pool.mu.Unlock()

	if s.isExpired() {
		t.Error("isExpired: fresh session should not be expired")
	}
}

func TestSession_IsExpired_AfterTTL(t *testing.T) {
	pool := NewSessionPool(50*time.Millisecond, 0)
	defer pool.Close()

	id, _ := pool.Create("proxy")

	time.Sleep(120 * time.Millisecond)

	pool.mu.Lock()
	s := pool.sessions[id]
	pool.mu.Unlock()

	if !s.isExpired() {
		t.Error("isExpired: session past TTL should be expired")
	}
}

func TestSession_IsExpired_ZeroTTL(t *testing.T) {
	pool := NewSessionPool(0, 0)
	defer pool.Close()

	id, _ := pool.Create("proxy")

	pool.mu.Lock()
	s := pool.sessions[id]
	pool.mu.Unlock()

	if s.isExpired() {
		t.Error("isExpired: zero TTL means never expires")
	}
}

func TestSessionPool_Get_EagerlyEvictsExpired(t *testing.T) {
	pool := NewSessionPool(50*time.Millisecond, 0)
	defer pool.Close()

	id, _ := pool.Create("proxy")

	// Wait past the TTL — do NOT call evictExpired.
	time.Sleep(120 * time.Millisecond)

	_, err := pool.Get(id)
	if err == nil {
		t.Fatal("Get: expected error for expired session, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("Get: error should mention 'expired', got: %v", err)
	}

	// Session must have been removed eagerly.
	if pool.Count() != 0 {
		t.Errorf("Get: expired session should have been removed from pool, count = %d", pool.Count())
	}
}

func TestSessionPool_Get_NotFound(t *testing.T) {
	pool := NewSessionPool(time.Minute, 0)
	defer pool.Close()

	_, err := pool.Get("nonexistent-id")
	if err == nil {
		t.Fatal("Get: expected error for unknown session, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Get: error should mention 'not found', got: %v", err)
	}
}
