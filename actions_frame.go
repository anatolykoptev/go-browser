package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// execTypeViaKeyboard types text using Input.dispatchKeyEvent on the main page.
// Used when frame context is unavailable (OOP iframe) — relies on the iframe
// already having focus (via clickIframeArea). CDP keyboard events go to
// whatever element is focused, regardless of frame boundary.
func execTypeViaKeyboard(ctx context.Context, page *rod.Page, a Action) ActionResult {
	for _, ch := range a.Text {
		char := string(ch)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: char,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeChar, Text: char, UnmodifiedText: char,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeKeyUp, Key: char,
		}).Call(page)
		select {
		case <-ctx.Done():
			return ActionResult{Action: a.Type, Ok: false, Error: ctx.Err().Error()}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if a.Submit {
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: "Enter", Code: "Enter",
			WindowsVirtualKeyCode: 13,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeKeyUp, Key: "Enter", Code: "Enter",
		}).Call(page)
	}
	return ActionResult{Action: a.Type, Ok: true}
}

const (
	frameRetryInterval = 500 * time.Millisecond
	frameMaxRetries    = 10 // 10 × 500ms = 5 seconds max wait for iframe
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
// Retries with polling until ctx deadline if iframe is not immediately available.
func resolveFrame(ctx context.Context, page *rod.Page, selector string) (*rod.Page, error) {
	kind, pattern := parseFrameSelector(selector)

	var lastErr error
	for attempt := range frameMaxRetries {
		var frame *rod.Page
		var err error
		switch kind {
		case "url":
			frame, err = resolveFrameByURL(ctx, page, pattern)
		default:
			frame, err = resolveFrameByCSS(ctx, page, pattern)
		}
		if err == nil {
			return frame, nil
		}
		lastErr = err

		// Don't sleep after last attempt.
		if attempt == frameMaxRetries-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("frame %q: %w (last: %v)", selector, ctx.Err(), lastErr)
		case <-time.After(frameRetryInterval):
		}
	}
	return nil, fmt.Errorf("frame %q: not found after %d retries (last: %v)", selector, frameMaxRetries, lastErr)
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

// clickIframeArea clicks the center of an iframe element to transfer focus inside it.
// Used as fallback when el.Frame() fails for OOP cross-origin iframes.
func clickIframeArea(ctx context.Context, page *rod.Page, selector string) error {
	kind, pattern := parseFrameSelector(selector)

	var el *rod.Element
	var err error

	switch kind {
	case "url":
		// Find iframe by src URL match.
		els, findErr := page.Context(ctx).Elements("iframe")
		if findErr != nil {
			return fmt.Errorf("click_iframe url=%q: %w", pattern, findErr)
		}
		for _, candidate := range els {
			src, _ := candidate.Attribute("src")
			if src != nil && matchFrameURL(*src, pattern) {
				el = candidate
				break
			}
		}
		if el == nil {
			return fmt.Errorf("click_iframe url=%q: no matching iframe", pattern)
		}
	default:
		el, err = page.Context(ctx).Element(pattern)
		if err != nil {
			return fmt.Errorf("click_iframe %q: %w", pattern, err)
		}
	}

	return el.Click(proto.InputMouseButtonLeft, 1)
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
