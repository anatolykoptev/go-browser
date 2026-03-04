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

// WithConcurrency sets maximum concurrent renders.
func WithConcurrency(n int) Option {
	return func(o *Options) { o.Concurrency = n }
}

// WithRenderTimeout sets per-page render deadline.
func WithRenderTimeout(d time.Duration) Option {
	return func(o *Options) { o.RenderTimeout = d }
}

// WithHydrationWait sets the delay after body ready before capturing HTML.
func WithHydrationWait(d time.Duration) Option {
	return func(o *Options) { o.HydrationWait = d }
}

// WithUserAgent overrides the browser User-Agent header.
func WithUserAgent(ua string) Option {
	return func(o *Options) { o.UserAgent = ua }
}

// Apply applies options over defaults.
func Apply(opts ...Option) Options {
	o := DefaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
