package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func init() {
	registerAction("type_into_frame", execTypeIntoFrame)
}

// execTypeIntoFrame is an atomic action that clicks an iframe to focus it,
// then types text via CDP keyboard events — all in one action, no round-trip.
// Use when frame_selector + type_text fails due to OOP iframe timing issues.
//
// Required fields: frame_selector (CSS or url=), text.
// Optional: selector (CSS selector to click inside iframe before typing — requires frame access).
// If selector is empty, clicks the iframe element center and types immediately.
func execTypeIntoFrame(dc dispatchContext, a Action) (any, error) {
	if a.FrameSelector == "" {
		return nil, fmt.Errorf("type_into_frame: frame_selector is required")
	}
	if a.Text == "" {
		return nil, fmt.Errorf("type_into_frame: text is required")
	}

	// Click the iframe element in parent page to transfer focus inside.
	if err := clickIframeArea(dc.ctx, dc.page, a.FrameSelector); err != nil {
		return nil, fmt.Errorf("type_into_frame: %w", err)
	}

	// Small delay for focus transfer.
	select {
	case <-dc.ctx.Done():
		return nil, dc.ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	// Type via CDP keyboard — goes to whatever is focused.
	if err := typeViaKeyboard(dc.ctx, dc.page, a.Text); err != nil {
		return nil, fmt.Errorf("type_into_frame: %w", err)
	}

	if a.Submit {
		pressKey(dc.page, "Enter", 13)
	}

	return nil, nil
}

// typeViaKeyboard sends text character-by-character via Input.dispatchKeyEvent.
func typeViaKeyboard(ctx context.Context, page *rod.Page, text string) error {
	for _, ch := range text {
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
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return nil
}

func pressKey(page *rod.Page, key string, vk int) {
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: key, Code: key,
		WindowsVirtualKeyCode: vk,
	}).Call(page)
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeKeyUp, Key: key, Code: key,
	}).Call(page)
}
