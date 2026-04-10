package cdputil

import (
	"fmt"
	"math/rand/v2"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// ClickNode clicks the center of a DOM node using CDP Input.dispatchMouseEvent.
// Does NOT use Runtime.callFunctionOn — safe for PX-protected pages.
func ClickNode(page *rod.Page, nodeID NodeID, btn proto.InputMouseButton, clickCount int) error {
	x, y, err := NodeCenter(page, nodeID)
	if err != nil {
		return fmt.Errorf("click: %w", err)
	}

	// Small random offset (±3px) for human-like behavior.
	x += float64(rand.IntN(7) - 3)
	y += float64(rand.IntN(7) - 3)

	if clickCount <= 0 {
		clickCount = 1
	}

	_ = (proto.InputDispatchMouseEvent{
		Type:   proto.InputDispatchMouseEventTypeMouseMoved,
		X:      x,
		Y:      y,
		Button: btn,
	}).Call(page)

	_ = (proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMousePressed,
		X:          x,
		Y:          y,
		Button:     btn,
		ClickCount: clickCount,
	}).Call(page)

	_ = (proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMouseReleased,
		X:          x,
		Y:          y,
		Button:     btn,
		ClickCount: clickCount,
	}).Call(page)

	return nil
}

// NodeCenter returns the center (x, y) of a DOM node's content box.
func NodeCenter(page *rod.Page, nodeID NodeID) (float64, float64, error) {
	box, err := (proto.DOMGetBoxModel{NodeID: nodeID}).Call(page)
	if err != nil {
		return 0, 0, fmt.Errorf("DOM.getBoxModel: %w", err)
	}
	q := box.Model.Content
	if len(q) < 8 {
		return 0, 0, fmt.Errorf("invalid content quad (len=%d)", len(q))
	}
	cx := (q[0] + q[2] + q[4] + q[6]) / 4
	cy := (q[1] + q[3] + q[5] + q[7]) / 4
	return cx, cy, nil
}

// FocusNode focuses a DOM node using CDP DOM.focus.
func FocusNode(page *rod.Page, nodeID NodeID) error {
	err := (proto.DOMFocus{NodeID: nodeID}).Call(page)
	if err == nil {
		return nil
	}
	// Fallback for contenteditable divs and other non-focusable elements:
	// click the center of the element to focus it.
	return ClickNode(page, nodeID, proto.InputMouseButtonLeft, 1)
}

// ScrollIntoView scrolls a node into the viewport using CDP DOM.scrollIntoViewIfNeeded.
func ScrollIntoView(page *rod.Page, nodeID NodeID) error {
	if err := (proto.DOMScrollIntoViewIfNeeded{NodeID: nodeID}).Call(page); err != nil {
		return fmt.Errorf("DOM.scrollIntoViewIfNeeded: %w", err)
	}
	return nil
}
