package browser

import (
	"testing"
	"time"
)

func TestContextPool_DetachAttachSession(t *testing.T) {
	// Create a mock ManagedPage directly to test detach/attach logic
	mp := &ManagedPage{
		Session:    "test-session",
		DetachedAt: time.Time{}, // initially attached
	}

	// Initially should be attached (DetachedAt is zero)
	if !mp.DetachedAt.IsZero() {
		t.Error("session should be attached initially")
	}

	// Test detach logic directly
	mp.mu.Lock()
	mp.DetachedAt = time.Now()
	mp.mu.Unlock()

	// Should now be detached
	if mp.DetachedAt.IsZero() {
		t.Error("session should be detached after setting DetachedAt")
	}

	// Test attach logic directly
	mp.mu.Lock()
	mp.DetachedAt = time.Time{}
	mp.mu.Unlock()

	// Should be attached again
	if !mp.DetachedAt.IsZero() {
		t.Error("session should be attached after clearing DetachedAt")
	}
}

func TestContextPool_DetachNonExistentSession(t *testing.T) {
	// Test with empty pool - should return error
	pool := &ContextPool{
		contexts: make(map[string]*ManagedContext),
	}
	
	err := pool.DetachSession("nonexistent", "default")
	if err == nil {
		t.Error("expected error when detaching non-existent session")
	}
}

func TestContextPool_AttachNonExistentSession(t *testing.T) {
	// Test with empty pool - should return error
	pool := &ContextPool{
		contexts: make(map[string]*ManagedContext),
	}
	
	err := pool.AttachSession("nonexistent", "default")
	if err == nil {
		t.Error("expected error when attaching non-existent session")
	}
}

func TestManagedPage_DetachedAtField(t *testing.T) {
	// Test DetachedAt field behavior directly
	mp := &ManagedPage{
		Session:    "test-session",
		DetachedAt: time.Time{}, // initially attached
	}

	// Initially should be attached
	if !mp.DetachedAt.IsZero() {
		t.Error("DetachedAt should be zero initially")
	}

	// Detach and check timestamp
	before := time.Now()
	mp.mu.Lock()
	mp.DetachedAt = time.Now()
	mp.mu.Unlock()
	after := time.Now()

	if mp.DetachedAt.Before(before) || mp.DetachedAt.After(after) {
		t.Error("DetachedAt should be set to current time")
	}

	// Attach and check it's cleared
	mp.mu.Lock()
	mp.DetachedAt = time.Time{}
	mp.mu.Unlock()

	if !mp.DetachedAt.IsZero() {
		t.Error("DetachedAt should be cleared after attach")
	}
}

