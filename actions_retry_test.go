package browser

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithRetry_SucceedsOnThirdAttempt(t *testing.T) {
	t.Parallel()

	attempts := 0
	sentinel := errors.New("transient error")

	err := withRetry(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return sentinel
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithRetry_ExhaustsAllAttempts(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("persistent error")
	attempts := 0

	err := withRetry(context.Background(), func() error {
		attempts++
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
	if attempts != retryAttempts {
		t.Fatalf("expected %d attempts, got %d", retryAttempts, attempts)
	}
}

func TestWithRetry_StopsOnContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	sentinel := errors.New("error")

	// Cancel the context after first attempt fires.
	// We use a channel to synchronize: cancel after fn returns error.
	ready := make(chan struct{})

	go func() {
		<-ready
		cancel()
	}()

	err := withRetry(ctx, func() error {
		attempts++
		close(ready) // signal cancellation after first call
		return sentinel
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should not have retried more than once (context was cancelled before second sleep completed).
	if attempts > 2 {
		t.Fatalf("expected at most 2 attempts after cancellation, got %d", attempts)
	}
}

func TestWithRetry_ReturnsImmediatelyOnSuccess(t *testing.T) {
	t.Parallel()

	start := time.Now()

	err := withRetry(context.Background(), func() error {
		return nil
	})

	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	// No delay should occur on first success — allow 50ms for scheduling noise.
	if elapsed > 50*time.Millisecond {
		t.Fatalf("expected immediate return, took %v", elapsed)
	}
}
