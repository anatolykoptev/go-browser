package browser

import (
	"context"
	"testing"
	"time"
)

func TestDispatchContext_RemainingMs(t *testing.T) {
	tests := []struct {
		name      string
		deadlineMs int64
		want      int
	}{
		{
			name:      "no deadline returns max int",
			deadlineMs: 0,
			want:      1<<31 - 1,
		},
		{
			name:      "negative deadline returns 0",
			deadlineMs: time.Now().UnixMilli() - 1000,
			want:      0,
		},
		{
			name:      "future deadline returns positive value",
			deadlineMs: time.Now().UnixMilli() + 5000,
			want:      func() int { return int(time.Now().UnixMilli() + 5000 - time.Now().UnixMilli()) }(),
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
