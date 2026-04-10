package browser

import (
	"context"
	"fmt"

	"github.com/go-rod/rod"
)

// findByText finds a clickable element containing the given text using XPath.
// Uses contains(.,text) to match text anywhere in the subtree (not just direct text nodes).
// Tries interactive ancestors first (label, li, button, a), then falls back to any container.
func findByText(ctx context.Context, page *rod.Page, text string) (*rod.Element, error) {
	p := page.Context(ctx)

	// Phase 1: find a clickable ancestor containing the text (label > li > button > a).
	// Uses "." instead of "text()" to match text in any descendant.
	clickable := fmt.Sprintf(
		`//*[contains(.,"%s")][self::label or self::li or self::button or self::a][not(ancestor::*[contains(.,"%s")][self::label or self::li or self::button or self::a])]`,
		text, text,
	)
	el, err := p.ElementX(clickable)
	if err == nil {
		return el, nil
	}

	// Phase 2: find any element with the text (span, div, td, etc.).
	any := fmt.Sprintf(
		`//*[contains(.,"%s")][not(self::html or self::body or self::head)][not(*[contains(.,"%s")])]`,
		text, text,
	)
	el, err = p.ElementX(any)
	if err == nil {
		return el, nil
	}

	return nil, fmt.Errorf("no element containing %q", text)
}
