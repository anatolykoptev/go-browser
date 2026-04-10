package browser

import (
	"context"
	"math/rand"
	"time"
)

const (
	retryAttempts = 3
	retryBaseMs   = 200
	retryJitterMs = 100
)

// withRetry executes fn up to retryAttempts times with jittered delay.
// Returns the first successful result or the last error.
func withRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for i := 0; i < retryAttempts; i++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if i < retryAttempts-1 {
			jitter := time.Duration(retryBaseMs+rand.Intn(retryJitterMs)) * time.Millisecond
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(jitter):
			}
		}
	}
	return lastErr
}
