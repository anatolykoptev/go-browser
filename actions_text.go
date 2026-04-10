package browser

import (
	"context"
	"fmt"

	"github.com/go-rod/rod"
)

// findByText finds a clickable element containing the given text using XPath.
// It prefers interactive elements (a, button, input, label, li, span, div)
// over generic text nodes to ensure the returned element can be clicked.
func findByText(ctx context.Context, page *rod.Page, text string) (*rod.Element, error) {
	// XPath: find element containing text, prefer clickable elements first.
	xpath := fmt.Sprintf(
		`//*[contains(text(),"%s")][self::a or self::button or self::label or self::input or self::li or self::span or self::div]`,
		text,
	)
	el, err := page.Context(ctx).ElementX(xpath)
	if err != nil {
		return nil, fmt.Errorf("no clickable element containing %q", text)
	}
	return el, nil
}
