package browser

import "time"

// Options holds common configuration for all backends.
type Options struct {
	Concurrency   int
	RenderTimeout time.Duration
	HydrationWait time.Duration
	UserAgent     string
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		Concurrency:   3,
		RenderTimeout: 20 * time.Second,
		HydrationWait: 2 * time.Second,
	}
}

// Option configures browser behavior.
type Option func(*Options)

// Apply applies options over defaults.
func Apply(opts ...Option) Options {
	o := DefaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
