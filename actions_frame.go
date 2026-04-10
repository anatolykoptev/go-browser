package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// parseFrameSelector returns ("css", selector) or ("url", pattern).
func parseFrameSelector(sel string) (string, string) {
	if strings.HasPrefix(sel, "url=") {
		return "url", strings.TrimPrefix(sel, "url=")
	}
	return "css", sel
}

// matchFrameURL returns true if frameURL contains the pattern (case-insensitive).
func matchFrameURL(frameURL, pattern string) bool {
	return strings.Contains(strings.ToLower(frameURL), strings.ToLower(pattern))
}

// resolveFrame finds an iframe and returns its rod.Page context.
// Supports CSS selectors and url=pattern (matches frame URL via Page.getFrameTree).
func resolveFrame(ctx context.Context, page *rod.Page, selector string) (*rod.Page, error) {
	kind, pattern := parseFrameSelector(selector)

	switch kind {
	case "url":
		return resolveFrameByURL(ctx, page, pattern)
	default:
		return resolveFrameByCSS(ctx, page, pattern)
	}
}

// resolveFrameByCSS finds an iframe element by CSS selector and enters its frame context.
func resolveFrameByCSS(ctx context.Context, page *rod.Page, selector string) (*rod.Page, error) {
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

// resolveFrameByURL finds a frame whose URL contains the pattern using Page.getFrameTree.
func resolveFrameByURL(ctx context.Context, page *rod.Page, pattern string) (*rod.Page, error) {
	tree, err := proto.PageGetFrameTree{}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("frame url=%q: get frame tree: %w", pattern, err)
	}

	frameID := findFrameByURL(tree.FrameTree, pattern)
	if frameID == "" {
		return nil, fmt.Errorf("frame url=%q: no frame found matching URL pattern", pattern)
	}

	// Find the iframe element in the parent page that hosts this frame.
	els, err := page.Context(ctx).Elements("iframe")
	if err != nil {
		return nil, fmt.Errorf("frame url=%q: enumerate iframes: %w", pattern, err)
	}
	for _, el := range els {
		src, _ := el.Attribute("src")
		if src != nil && matchFrameURL(*src, pattern) {
			frame, err := el.Frame()
			if err != nil {
				return nil, fmt.Errorf("frame url=%q: enter frame: %w", pattern, err)
			}
			return frame, nil
		}
	}

	return nil, fmt.Errorf("frame url=%q: frame found in tree (id=%s) but no matching iframe element", pattern, frameID)
}

// findFrameByURL recursively walks the frame tree to find a frame whose URL matches.
func findFrameByURL(tree *proto.PageFrameTree, pattern string) proto.PageFrameID {
	for _, child := range tree.ChildFrames {
		if matchFrameURL(child.Frame.URL, pattern) {
			return child.Frame.ID
		}
		if found := findFrameByURL(child, pattern); found != "" {
			return found
		}
	}
	return ""
}
