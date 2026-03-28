package browser

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
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

// doTypeTextHumanized types text character by character with human-like delays,
// dispatching real keydown/char/keyup CDP events so keyboard listeners see authentic input.
func doTypeTextHumanized(
	ctx context.Context, page *rod.Page, selector, text string, cursor *humanize.Cursor,
) error {
	if err := doClickHumanized(ctx, page, selector, cursor); err != nil {
		return fmt.Errorf("type_text: focus: %w", err)
	}

	delays := humanize.TypingDelays(text)
	for i, ch := range text {
		char := string(ch)
		code := charToCode(ch)
		vk := charToVK(ch)

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeRawKeyDown,
			Key:                   char,
			Code:                  code,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeChar,
			Text:                  char,
			UnmodifiedText:        char,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeKeyUp,
			Key:                   char,
			Code:                  code,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		if i < len(delays) {
			sleepCtx(ctx, time.Duration(delays[i])*time.Millisecond)
		}
	}
	return nil
}

// charToVK maps a character to its Windows Virtual Key code.
func charToVK(ch rune) int {
	switch {
	case ch >= 'a' && ch <= 'z':
		return int(ch - 32) // VK_A=65 .. VK_Z=90
	case ch >= 'A' && ch <= 'Z':
		return int(ch)
	case ch >= '0' && ch <= '9':
		return int(ch) // VK_0=48 .. VK_9=57
	case ch == ' ':
		return 32 // VK_SPACE
	case ch == '.':
		return 190 // VK_OEM_PERIOD
	case ch == ',':
		return 188 // VK_OEM_COMMA
	case ch == '-':
		return 189 // VK_OEM_MINUS
	case ch == '=':
		return 187 // VK_OEM_PLUS
	case ch == '@':
		return 50 // Shift+2
	case ch == '_':
		return 189 // Shift+Minus
	case ch == '!':
		return 49 // Shift+1
	case ch == '/':
		return 191 // VK_OEM_2
	case ch == ':':
		return 186 // Shift+;
	case ch == ';':
		return 186 // VK_OEM_1
	default:
		return int(ch)
	}
}

// charToCode maps a character to its DOM KeyboardEvent.code value.
func charToCode(ch rune) string {
	switch {
	case ch >= 'a' && ch <= 'z':
		return "Key" + strings.ToUpper(string(ch))
	case ch >= 'A' && ch <= 'Z':
		return "Key" + string(ch)
	case ch >= '0' && ch <= '9':
		return "Digit" + string(ch)
	case ch == ' ':
		return "Space"
	case ch == '.':
		return "Period"
	case ch == ',':
		return "Comma"
	case ch == '-':
		return "Minus"
	case ch == '=':
		return "Equal"
	case ch == '@':
		return "Digit2"
	case ch == '_':
		return "Minus"
	case ch == '!':
		return "Digit1"
	default:
		return ""
	}
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

// doWarmup generates realistic CDP mouse movements, scrolls, and clicks for the given
// duration. All events are native CDP Input dispatches (isTrusted: true) — not JS
// dispatchEvent — so fingerprinting SDKs like Castle.io register them as real user activity.
func doWarmup(ctx context.Context, page *rod.Page, durationMs int, cursor *humanize.Cursor) (int, error) {
	if durationMs <= 0 {
		durationMs = 3000
	}
	deadline := time.Now().Add(time.Duration(durationMs) * time.Millisecond)
	eventCount := 0
	vw := 1920.0
	vh := 1080.0

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return eventCount, ctx.Err()
		default:
		}

		// Random target within viewport
		targetX := 100 + rand.Float64()*(vw-200)
		targetY := 100 + rand.Float64()*(vh-200)

		// Bezier mouse move (5-10 steps, faster than normal)
		steps := 5 + rand.IntN(6)
		startX, startY := cursor.Position()
		path := humanize.BezierPath(startX, startY, targetX, targetY, steps)
		for _, p := range path {
			_ = proto.InputDispatchMouseEvent{
				Type: proto.InputDispatchMouseEventTypeMouseMoved,
				X:    p.X, Y: p.Y,
			}.Call(page)
			cursor.MoveTo(p.X, p.Y)
			eventCount++
		}
		sleepCtx(ctx, time.Duration(30+rand.IntN(70))*time.Millisecond)

		// Occasional scroll (20% chance)
		if rand.Float64() < 0.2 {
			_ = page.Mouse.Scroll(0, float64(50-rand.IntN(100)), 1)
			eventCount++
		}

		// Occasional click (10% chance)
		if rand.Float64() < 0.1 {
			_ = dispatchMouseClick(page, targetX, targetY)
			eventCount += 2
		}
	}

	return eventCount, nil
}

// sleepCtx sleeps for d, respecting context cancellation.
func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
