// Package browser provides a unified interface for headless browser automation.
package browser

import "context"

// Browser renders web pages via a headless browser backend.
type Browser interface {
	// Render navigates to url, waits for JS hydration, returns rendered page.
	Render(ctx context.Context, url string) (*Page, error)

	// Available reports whether the backend is connected and usable.
	Available() bool

	// Close shuts down the browser and releases resources.
	Close() error
}

// Page holds the result of a rendered page.
type Page struct {
	URL    string // Final URL after redirects.
	HTML   string // Rendered outerHTML.
	Title  string // Page title.
	Status int    // HTTP status code (0 if unknown).
}
