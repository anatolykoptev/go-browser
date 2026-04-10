package browser

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// doClickHumanized performs a human-like click: bezier mouse path to element, then click.
func doClickHumanized(ctx context.Context, page *rod.Page, selector string, cursor *humanize.Cursor) error {
	el, err := page.Context(ctx).Element(selector)
	if err != nil {
		return fmt.Errorf("click: find %q: %w", selector, err)
	}
	if err := el.ScrollIntoView(); err != nil {
		return fmt.Errorf("click: scroll into view: %w", err)
	}

	shape, err := el.Shape()
	if err != nil {
		return fmt.Errorf("click: shape: %w", err)
	}
	box := shape.Box()
	targetX := box.X + box.Width/2 + (rand.Float64()-0.5)*box.Width*0.3
	targetY := box.Y + box.Height/2 + (rand.Float64()-0.5)*box.Height*0.3

	if err := moveMouseBezier(ctx, page, cursor, targetX, targetY); err != nil {
		return fmt.Errorf("click: %w", err)
	}

	// Pre-click dwell: visual acquisition delay (T3 TMX behavioral biometric).
	// Dispatch micro-movements so the browser sees hand-settling jitter.
	dwell, microMoves := humanize.DwellDelay(box.Width)
	perMoveDelay := time.Duration(dwell/len(microMoves)) * time.Millisecond
	for _, mm := range microMoves {
		ev := proto.InputDispatchMouseEvent{
			Type: proto.InputDispatchMouseEventTypeMouseMoved,
			X:    targetX + mm.X,
			Y:    targetY + mm.Y,
		}
		_ = ev.Call(page)
		sleepCtx(ctx, perMoveDelay)
	}

	if err := dispatchMouseClick(page, targetX, targetY); err != nil {
		return fmt.Errorf("click: %w", err)
	}

	return nil
}

// dispatchMouseClick sends CDP press+release events at the given coordinates.
func dispatchMouseClick(page *rod.Page, x, y float64) error {
	press := proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMousePressed,
		X:          x,
		Y:          y,
		Button:     proto.InputMouseButtonLeft,
		ClickCount: 1,
	}
	if err := press.Call(page); err != nil {
		return fmt.Errorf("mouse press: %w", err)
	}

	release := proto.InputDispatchMouseEvent{
		Type:       proto.InputDispatchMouseEventTypeMouseReleased,
		X:          x,
		Y:          y,
		Button:     proto.InputMouseButtonLeft,
		ClickCount: 1,
	}
	if err := release.Call(page); err != nil {
		return fmt.Errorf("mouse release: %w", err)
	}
	return nil
}

// doHoverHumanized moves mouse to element via bezier path without clicking.
func doHoverHumanized(ctx context.Context, page *rod.Page, selector string, cursor *humanize.Cursor) error {
	el, err := page.Context(ctx).Element(selector)
	if err != nil {
		return fmt.Errorf("hover: find %q: %w", selector, err)
	}
	if err := el.ScrollIntoView(); err != nil {
		return fmt.Errorf("hover: scroll into view: %w", err)
	}

	shape, err := el.Shape()
	if err != nil {
		return fmt.Errorf("hover: shape: %w", err)
	}
	box := shape.Box()
	targetX := box.X + box.Width/2
	targetY := box.Y + box.Height/2

	if err := moveMouseBezier(ctx, page, cursor, targetX, targetY); err != nil {
		return fmt.Errorf("hover: %w", err)
	}

	return nil
}

// moveMouseBezier moves the mouse from cursor's current position to (targetX, targetY)
// along a Bezier curve, dispatching CDP mouse-move events at each step.
func moveMouseBezier(
	ctx context.Context, page *rod.Page, cursor *humanize.Cursor, targetX, targetY float64,
) error {
	startX, startY := cursor.Position()
	steps := 15 + rand.IntN(10)
	path := humanize.BezierPath(startX, startY, targetX, targetY, steps)

	for _, p := range path {
		ev := proto.InputDispatchMouseEvent{
			Type: proto.InputDispatchMouseEventTypeMouseMoved,
			X:    p.X,
			Y:    p.Y,
		}
		if err := ev.Call(page); err != nil {
			return fmt.Errorf("mouse move: %w", err)
		}
		cursor.MoveTo(p.X, p.Y)
		sleepCtx(ctx, time.Duration(humanize.MouseDelay())*time.Millisecond)
	}
	return nil
}
