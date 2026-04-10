package browser

import (
	"testing"
	"time"
)

func TestSession_RefMapPersistence(t *testing.T) {
	pool := NewSessionPool(5*time.Minute, 10)
	defer pool.Close()

	id, err := pool.Create("proxy1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Store a ref in one Get() call.
	sess, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	sess.Refs.Store("e1", 42)

	// Resolve the same ref in a separate Get() call.
	sess2, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get (second): %v", err)
	}
	got, ok := sess2.Refs.Resolve("e1")
	if !ok || got != 42 {
		t.Fatalf("Resolve(e1) = %d, %v; want 42, true", got, ok)
	}
}

func TestSession_RefMapClear(t *testing.T) {
	pool := NewSessionPool(5*time.Minute, 10)
	defer pool.Close()

	id, err := pool.Create("proxy1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sess, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	sess.Refs.Store("e1", 100)
	sess.Refs.Clear()

	sess2, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get (after clear): %v", err)
	}
	_, ok := sess2.Refs.Resolve("e1")
	if ok {
		t.Fatal("after Clear, Resolve should return false")
	}
}

func TestSession_RefMapInitialized(t *testing.T) {
	pool := NewSessionPool(5*time.Minute, 10)
	defer pool.Close()

	id, err := pool.Create("proxy1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sess, err := pool.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess.Refs == nil {
		t.Fatal("Refs should be initialized, got nil")
	}
}
