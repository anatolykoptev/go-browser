package remote

import browser "github.com/anatolykoptev/go-browser"

// Options for remote CDP backend.
type Options struct {
	browser.Options
	Endpoint string // WebSocket URL (e.g. ws://browserless:3000).
}

// Option configures the remote backend.
type Option func(*Options)

// WithEndpoint sets the CDP WebSocket endpoint.
func WithEndpoint(url string) Option {
	return func(o *Options) { o.Endpoint = url }
}
