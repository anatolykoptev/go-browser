package browser

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// doWarmup generates realistic CDP mouse movements, scrolls, and clicks for the given
// duration. All events are native CDP Input dispatches (isTrusted: true) — not JS
// dispatchEvent — so fingerprinting SDKs like Castle.io register them as real user activity.
func doWarmup(ctx context.Context, page *rod.Page, durationMs int, cursor *humanize.Cursor) (int, error) {
	if durationMs <= 0 {
		durationMs = defaultWarmupMs
	}
	deadline := time.Now().Add(time.Duration(durationMs) * time.Millisecond)
	eventCount := 0

	// Read actual viewport dimensions; fall back to 1440×900 on error.
	vw, vh, _ := humanize.ReadViewport(page)
	bounds := humanize.ViewportBounds{
		MinX: 50, MaxX: vw - 50,
		MinY: 50, MaxY: vh - 50,
	}

	// dispatch wraps CDP mouse-move for idle drift.
	dispatch := func(x, y float64) error {
		return proto.InputDispatchMouseEvent{
			Type: proto.InputDispatchMouseEventTypeMouseMoved,
			X:    x, Y: y,
		}.Call(page)
	}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return eventCount, ctx.Err()
		default:
		}

		// Random target within viewport margins.
		targetX := bounds.MinX + rand.Float64()*(bounds.MaxX-bounds.MinX)
		targetY := bounds.MinY + rand.Float64()*(bounds.MaxY-bounds.MinY)

		// Bezier mouse move (5-10 steps, faster than normal).
		steps := 5 + rand.IntN(6)
		startX, startY := cursor.Position()
		path := humanize.BezierPath(startX, startY, targetX, targetY, steps)
		for _, p := range path {
			_ = dispatch(p.X, p.Y)
			cursor.MoveTo(p.X, p.Y)
			eventCount++
		}

		// Idle drift pause between Bezier sequences (500ms-1s).
		driftDuration := time.Duration(500+rand.IntN(500)) * time.Millisecond
		driftCtx, driftStop := context.WithTimeout(ctx, driftDuration)
		stopDrift := humanize.StartIdleDrift(driftCtx, cursor, dispatch)
		sleepCtx(driftCtx, driftDuration)
		stopDrift()
		driftStop()

		// Occasional scroll (20% chance).
		if rand.Float64() < 0.2 {
			_ = page.Mouse.Scroll(0, float64(50-rand.IntN(100)), 1)
			eventCount++
		}

		// Occasional click (10% chance).
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
