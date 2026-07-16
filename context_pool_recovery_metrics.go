package browser

import "sync/atomic"

// Stale-default-context recovery outcomes. The pool caches the default Chrome
// BrowserContextID; if Chrome disposes/recreates the default context out from
// under the pool, the cached handle goes stale and default-context page creation
// fails with a CDP "Failed to find browser context with id" error. The pool now
// detects that class, re-discovers the live default context, and retries once.
//
// go-browser has no Prometheus registry of its own (see metrics.go — CWV only),
// so this exposes process-lifetime counters via an exported snapshot that
// embedders (go-wowa, ox-browser) surface through their own metrics layer. The
// counter name mirrors the agreed series chrome_stale_context_recovery_total
// {outcome=detected|recovered|failed}.
const (
	// StaleCtxOutcomeDetected counts each time a stale-default-context CDP error
	// was observed on a default-context page creation.
	StaleCtxOutcomeDetected = "detected"
	// StaleCtxOutcomeRecovered counts each time the pool re-discovered the live
	// default context and the retried page creation succeeded.
	StaleCtxOutcomeRecovered = "recovered"
	// StaleCtxOutcomeFailed counts each time recovery was attempted but the
	// retried page creation still failed.
	StaleCtxOutcomeFailed = "failed"
)

var (
	staleCtxDetected  atomic.Uint64
	staleCtxRecovered atomic.Uint64
	staleCtxFailed    atomic.Uint64
)

func recordStaleCtxRecovery(outcome string) {
	switch outcome {
	case StaleCtxOutcomeDetected:
		staleCtxDetected.Add(1)
	case StaleCtxOutcomeRecovered:
		staleCtxRecovered.Add(1)
	case StaleCtxOutcomeFailed:
		staleCtxFailed.Add(1)
	}
}

// StaleContextRecoveryStats returns a process-lifetime snapshot of stale-default-context
// recovery events by outcome. Embedders (go-wowa, ox-browser) surface these through their
// own Prometheus metrics layer as chrome_stale_context_recovery_total {outcome=...}.
//
// The map is freshly allocated on each call to avoid race conditions on the returned slice.
func StaleContextRecoveryStats() map[string]uint64 {
	return map[string]uint64{
		StaleCtxOutcomeDetected:  staleCtxDetected.Load(),
		StaleCtxOutcomeRecovered: staleCtxRecovered.Load(),
		StaleCtxOutcomeFailed:    staleCtxFailed.Load(),
	}
}
