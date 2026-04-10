package browser

import (
	"context"
	"fmt"

	"github.com/go-rod/rod"
)

// resolveFrame finds an iframe by CSS selector and returns its rod.Page.
// Works with both same-origin and cross-origin iframes.
// The returned *rod.Page is the iframe's execution context, compatible with
// all existing action executors.
func resolveFrame(ctx context.Context, page *rod.Page, selector string) (*rod.Page, error) {
	el, err := page.Context(ctx).Element(selector)
	if err != nil {
		return nil, fmt.Errorf("frame %q: element not found: %w", selector, err)
	}

	frame, err := el.Frame()
	if err != nil {
		return nil, fmt.Errorf("frame %q: not an iframe or frame not accessible: %w", selector, err)
	}

	return frame, nil
}
