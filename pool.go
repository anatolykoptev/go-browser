package browser

import "context"

// Pool limits concurrent browser operations via a semaphore.
type Pool struct {
	sem  chan struct{}
	done chan struct{}
}

// NewPool creates a pool with the given concurrency limit.
func NewPool(size int) *Pool {
	if size < 1 {
		size = 1
	}
	return &Pool{
		sem:  make(chan struct{}, size),
		done: make(chan struct{}),
	}
}

// Acquire blocks until a slot is available or ctx is cancelled.
// Returns a release function that must be called when done.
func (p *Pool) Acquire(ctx context.Context) (release func(), err error) {
	select {
	case p.sem <- struct{}{}:
		return func() { <-p.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return nil, ErrUnavailable
	}
}

// Close signals all waiters to abort.
func (p *Pool) Close() {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
}
