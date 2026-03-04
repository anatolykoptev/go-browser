package rod

import (
	browser "github.com/anatolykoptev/go-browser"
	stealth "github.com/anatolykoptev/go-stealth"
)

// Options extends common browser options with Rod-specific settings.
type Options struct {
	browser.Options
	Bin       string                    // Custom Chromium binary path (empty = auto-download).
	ProxyPool stealth.ProxyPoolProvider // Rotating proxy pool from go-stealth.
	Headless  bool                      // Run in headless mode (default true).
}

// DefaultOptions returns Rod defaults.
func DefaultOptions() Options {
	return Options{
		Options:  browser.DefaultOptions(),
		Headless: true,
	}
}

// Option configures the Rod backend.
type Option func(*Options)

// WithBin sets a custom Chromium binary path.
func WithBin(path string) Option {
	return func(o *Options) { o.Bin = path }
}

// WithProxyPool sets the go-stealth proxy pool for rotation.
func WithProxyPool(pool stealth.ProxyPoolProvider) Option {
	return func(o *Options) { o.ProxyPool = pool }
}

// WithHeadless toggles headless mode (default true).
func WithHeadless(v bool) Option {
	return func(o *Options) { o.Headless = v }
}
