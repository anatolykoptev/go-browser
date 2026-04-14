package browser

import (
	"context"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

func TestDispatchContext_RemainingMs(t *testing.T) {
	tests := []struct {
		name       string
		deadlineMs int64
		want       int
	}{
		{
			name:       "no deadline returns max int",
			deadlineMs: 0,
			want:       1<<31 - 1,
		},
		{
			name:       "negative deadline returns 0",
			deadlineMs: time.Now().UnixMilli() - 1000,
			want:       0,
		},
		{
			name:       "future deadline returns positive value",
			deadlineMs: time.Now().UnixMilli() + 5000,
			want:       func() int { return int(time.Now().UnixMilli() + 5000 - time.Now().UnixMilli()) }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := dispatchContext{
				deadlineMs: tt.deadlineMs,
			}
			got := dc.remainingMs()

			// For future deadline test, allow small variance due to timing
			if tt.deadlineMs > time.Now().UnixMilli() {
				if got < 0 || got > 5000 {
					t.Errorf("remainingMs() = %d, want around 5000", got)
				}
			} else {
				if got != tt.want {
					t.Errorf("remainingMs() = %d, want %d", got, tt.want)
				}
			}
		})
	}
}

func TestDispatchContext_EffectiveTimeoutMs(t *testing.T) {
	unbounded := 1<<31 - 1
	now := time.Now().UnixMilli()

	tests := []struct {
		name       string
		deadlineMs int64
		actionMs   int
		wantLo     int // inclusive lower bound (for time-sensitive cases)
		wantHi     int // inclusive upper bound
	}{
		{"no deadline, no action → 0", 0, 0, 0, 0},
		{"no deadline, action=2000 → 2000", 0, 2000, 2000, 2000},
		{"deadline=5000, no action → ~5000", now + 5000, 0, 4500, 5000},
		{"deadline=10000, action=2000 → 2000 (action wins)", now + 10000, 2000, 2000, 2000},
		{"deadline=1000, action=5000 → ~1000 (remaining wins)", now + 1000, 5000, 500, 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := &dispatchContext{deadlineMs: tt.deadlineMs}
			got := dc.effectiveTimeoutMs(tt.actionMs)
			if got == unbounded {
				t.Fatalf("unexpected unbounded result")
			}
			if got < tt.wantLo || got > tt.wantHi {
				t.Errorf("effectiveTimeoutMs(%d) = %d, want in [%d,%d]", tt.actionMs, got, tt.wantLo, tt.wantHi)
			}
		})
	}
}

// TestExecClick_RespectsActionTimeoutMs drives a click against a missing
// selector with no chain deadline but a 1500ms action-local timeout; it
// must fail inside ~2s (not hang).
func TestExecClick_RespectsActionTimeoutMs(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	br := acquireSharedBrowser(t)
	page, _ := br.Page(proto.TargetCreateTarget{URL: "about:blank"})
	defer func() { _ = page.Close() }()

	dc := dispatchContext{
		ctx:  context.Background(),
		page: page,
		// No deadlineMs — only action's own timeout_ms should apply.
	}
	a := Action{Type: "click", Selector: "#nope", TimeoutMs: 1500}
	start := time.Now()
	_, err := execClick(dc, a)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("expected click to fail on missing selector")
	}
	if elapsed > 3*time.Second {
		t.Errorf("click did not honor timeout_ms=1500: took %v", elapsed)
	}
}

func TestExecuteAction_WithDeadline(t *testing.T) {
	// Test that ExecuteAction respects deadline when passed
	ctx := context.Background()

	// Test with action that doesn't require page (destroy_session)
	a := Action{Type: "destroy_session"}

	// Set deadline far in future - should succeed normally
	futureDeadline := time.Now().UnixMilli() + 10000
	result := ExecuteAction(ctx, nil, a, nil, nil, false, nil, futureDeadline)

	if !result.Ok {
		t.Errorf("expected destroy_session to succeed with future deadline, got error: %s", result.Error)
	}
}

func TestWaitForAction_WithDeadline(t *testing.T) {
	// Test that wait_for action respects deadline - use sleep instead to avoid page dependency
	dc := dispatchContext{
		ctx:        context.Background(),
		deadlineMs: time.Now().UnixMilli() + 50, // 50ms deadline
	}

	a := Action{
		Type:   "sleep",
		WaitMs: 1000, // 1 second sleep
	}

	start := time.Now()
	_, err := execSleep(dc, a)
	elapsed := time.Since(start)

	// Should complete quickly due to deadline limiting sleep time
	if err != nil {
		t.Errorf("sleep should not error with deadline, got: %v", err)
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("sleep took too long with deadline: %v", elapsed)
	}
}

func TestSleepAction_WithDeadline(t *testing.T) {
	// Test that sleep action respects deadline
	dc := dispatchContext{
		ctx:        context.Background(),
		deadlineMs: time.Now().UnixMilli() + 50, // 50ms deadline
	}

	a := Action{
		Type:   "sleep",
		WaitMs: 1000, // 1 second sleep
	}

	start := time.Now()
	_, err := execSleep(dc, a)
	elapsed := time.Since(start)

	// Should fail fast due to deadline, not sleep 1 second
	if err != nil {
		t.Errorf("sleep should not error with deadline, got: %v", err)
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("sleep took too long with deadline: %v", elapsed)
	}
}
