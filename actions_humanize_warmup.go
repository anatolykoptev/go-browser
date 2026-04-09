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
