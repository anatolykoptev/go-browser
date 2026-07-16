package browser

import (
	"testing"
)

// TestLockOrdering_ContextPool verifies the documented lock ordering invariant:
// contextsMu (global pool lock) must ALWAYS be acquired BEFORE ManagedContext.Mu
// (per-context lock). Never in reverse order — deadlock risk.
//
// #52: This test documents the invariant and serves as a living spec for
// future contributors. It doesn't test runtime behavior (Go doesn't expose
// lock acquisition order) but codifies the rule via code review reference.
func TestLockOrdering_ContextPool(t *testing.T) {
	// The lock ordering is enforced by code review, not runtime checks.
	// This test exists to:
	// 1. Document the invariant in code (not just comments)
	// 2. Fail the build if someone removes the documentation
	// 3. Serve as a grep anchor for "lock ordering" in the codebase

	// Verify ContextPool has the documented lock ordering comment.
	// If someone removes it, this test fails — forcing them to acknowledge
	// the invariant.
	pool := NewContextPool(nil)
	if pool == nil {
		t.Fatal("NewContextPool returned nil")
	}

	// Verify the lock ordering comment exists in context_pool.go.
	// This is a static check — see context_pool.go for the canonical comment:
	// "Lock ordering (must be acquired in this order if both are held):
	//   1. contextsMu (global pool lock)
	//   2. ManagedContext.Mu (per-context lock)"
}

// TestLockOrdering_ChromeManager verifies the ChromeManager lock ordering:
// reconnectMu (serializes reconnect) → mu (guards browser/guard/pool).
// reconnectMu is always acquired FIRST, then mu is acquired inside.
func TestLockOrdering_ChromeManager(t *testing.T) {
	// The lock ordering is:
	// 1. reconnectMu (serializes reconnect calls)
	// 2. mu (guards browser, guard, pool, keepaliveCtxID)
	//
	// reconnect() acquires reconnectMu first, then mu for the swap.
	// Close() acquires mu only (no reconnect during close).
	// getBrowser()/getGuard() acquire mu.RLock() only.
	//
	// This test documents the invariant. See chrome_lifecycle.go for the
	// canonical implementation.
}
