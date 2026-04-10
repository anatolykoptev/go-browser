package browser

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestWithRetry_NilFunction verifies that passing a nil fn panics rather than silently misbehaving.
// This documents the current behavior so callers know they must not pass nil.
func TestWithRetry_NilFunction(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when fn is nil, but did not panic")
		}
	}()

	//nolint:staticcheck // intentional nil function to document panic behavior
	_ = withRetry(context.Background(), nil)
}

// TestWithRetry_ImmediateContextCancel verifies that a pre-cancelled context causes
// withRetry to return after the first attempt without sleeping.
func TestWithRetry_ImmediateContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling withRetry

	attempts := 0
	sentinel := errors.New("fail")

	start := time.Now()
	err := withRetry(ctx, func() error {
		attempts++
		return sentinel
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// First attempt must always execute (withRetry calls fn before checking ctx).
	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt with pre-cancelled ctx, got %d", attempts)
	}
	// No delay should have been waited because ctx was already done.
	if elapsed > 50*time.Millisecond {
		t.Fatalf("expected near-immediate return with cancelled ctx, took %v", elapsed)
	}
}

// TestWithRetry_Timing verifies that 3 consecutive failures incur 2 delay intervals
// totalling between 400 ms and 700 ms (2 x [200..300] ms with scheduling slack).
func TestWithRetry_Timing(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("timing error")

	start := time.Now()
	_ = withRetry(context.Background(), func() error {
		return sentinel
	})
	elapsed := time.Since(start)

	// 2 delays x retryBaseMs + up to retryJitterMs each = 2x200..2x300 ms
	const minExpected = 2 * retryBaseMs * time.Millisecond
	const maxExpected = 2*(retryBaseMs+retryJitterMs)*time.Millisecond + 150*time.Millisecond // +150ms scheduling slack

	if elapsed < minExpected {
		t.Fatalf("timing too fast: %v < %v; delays may have been skipped", elapsed, minExpected)
	}
	if elapsed > maxExpected {
		t.Fatalf("timing too slow: %v > %v; unexpected extra delay", elapsed, maxExpected)
	}
}

// TestWithRetry_SucceedsFirstAttempt verifies that a function succeeding immediately
// causes withRetry to return without any retry.
func TestWithRetry_SucceedsFirstAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := withRetry(context.Background(), func() error {
		attempts++
		return nil // succeed immediately
	})

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt on immediate success, got %d", attempts)
	}
}

// TestWithRetry_PreservesOriginalError verifies that the exact last error is returned
// without wrapping or mutation.
func TestWithRetry_PreservesOriginalError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("exact sentinel message: xyz-42")

	err := withRetry(context.Background(), func() error {
		return sentinel
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Must be the exact same error, not a wrapped version.
	if !errors.Is(err, sentinel) {
		t.Fatalf("error identity lost; got %v (%T), want sentinel", err, err)
	}
	// Message must be preserved verbatim.
	if err.Error() != sentinel.Error() {
		t.Fatalf("error message changed: got %q, want %q", err.Error(), sentinel.Error())
	}
}
