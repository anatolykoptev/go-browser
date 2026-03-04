package rod

import (
	browser "github.com/anatolykoptev/go-browser"
	stealth "github.com/anatolykoptev/go-stealth"
)

// ResourceType identifies a network resource type for blocking.
type ResourceType string

// Resource types matching Chrome DevTools Protocol network resource types.
const (
	ResourceImage      ResourceType = "Image"
	ResourceFont       ResourceType = "Font"
	ResourceStylesheet ResourceType = "Stylesheet"
	ResourceMedia      ResourceType = "Media"
)

// Options extends common browser options with Rod-specific settings.
type Options struct {
	browser.Options
	Bin            string                    // Custom Chromium binary path (empty = auto-download).
	ProxyPool      stealth.ProxyPoolProvider // Rotating proxy pool from go-stealth.
	Headless       bool                      // Run in headless mode (default true).
	BlockResources []ResourceType            // Resource types to block (images, fonts, etc.).
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

// WithBlockResources configures resource types to block during rendering.
func WithBlockResources(types ...ResourceType) Option {
	return func(o *Options) { o.BlockResources = types }
}
