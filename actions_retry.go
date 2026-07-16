package browser

import (
	"context"
	"math/rand"
	"time"
)

const (
	retryAttempts = 3 // #30: max retry cap
	retryBaseMs   = 100
	retryJitterMs = 100
)

// withRetry executes fn up to retryAttempts times with exponential backoff.
// #30: Uses exponential backoff (100ms * 2^attempt) with jitter to avoid
// thundering-herd retries. Returns the first successful result or the last error.
func withRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for i := 0; i < retryAttempts; i++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if i < retryAttempts-1 {
			// #30: Exponential backoff — 100ms * 2^attempt plus jitter.
			backoff := time.Duration(retryBaseMs*(1<<i)+rand.Intn(retryJitterMs)) * time.Millisecond
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(backoff):
			}
		}
	}
	return lastErr
}
