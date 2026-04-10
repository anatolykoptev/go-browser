package browser

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const (
	browseMinDurationMs = 5000
	browseMaxElements   = 8
)

func init() {
	registerAction("warmup_browse", execWarmupBrowse)
}

// execWarmupBrowse simulates natural page browsing: scroll down, hover links,
// move between sections. Builds TMX behavioral history before payment.
// Uses real page elements (not random coordinates) for realistic patterns.
func execWarmupBrowse(dc dispatchContext, a Action) (any, error) {
	durationMs := a.WaitMs
	if durationMs < browseMinDurationMs {
		durationMs = browseMinDurationMs
	}

	count, err := doBrowseWarmup(dc.ctx, dc.page, durationMs, dc.cursor)
	return map[string]any{"events": count}, err
}

// doBrowseWarmup generates realistic browsing behavior:
// 1. Scroll down progressively (reading simulation)
// 2. Hover over real interactive elements
// 3. Move mouse between page sections
// 4. Occasional back-scroll (re-reading)
func doBrowseWarmup(ctx context.Context, page *rod.Page, durationMs int, cursor *humanize.Cursor) (int, error) {
	deadline := time.Now().Add(time.Duration(durationMs) * time.Millisecond)
	eventCount := 0

	// Find real interactive elements to hover.
	targets := findHoverTargets(page)

	// Phase 1: Initial scroll down (reading the page).
	scrollY := 0.0
	for i := 0; i < 5 && time.Now().Before(deadline); i++ {
		select {
		case <-ctx.Done():
			return eventCount, ctx.Err()
		default:
		}

		// Smooth scroll in small increments.
		delta := 80.0 + rand.Float64()*120.0
		_ = page.Mouse.Scroll(0, delta, 1)
		scrollY += delta
		eventCount++

		// Pause between scrolls (reading time).
		sleepCtx(ctx, time.Duration(400+rand.IntN(800))*time.Millisecond)
	}

	// Phase 2: Hover over real elements.
	if cursor != nil {
		for i, t := range targets {
			if i >= browseMaxElements || !time.Now().Before(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				return eventCount, ctx.Err()
			default:
			}

			// Move mouse to element center.
			startX, startY := cursor.Position()
			steps := 8 + rand.IntN(8)
			path := humanize.BezierPath(startX, startY, t.x, t.y, steps)
			for _, p := range path {
				_ = proto.InputDispatchMouseEvent{
					Type: proto.InputDispatchMouseEventTypeMouseMoved,
					X:    p.X, Y: p.Y,
				}.Call(page)
				cursor.MoveTo(p.X, p.Y)
				eventCount++
			}

			// Hover pause (reading link text).
			sleepCtx(ctx, time.Duration(200+rand.IntN(600))*time.Millisecond)
		}
	}

	// Phase 3: Scroll back up a bit (re-reading).
	if time.Now().Before(deadline) && scrollY > 200 {
		backScroll := 100.0 + rand.Float64()*150.0
		_ = page.Mouse.Scroll(0, -backScroll, 1)
		eventCount++
		sleepCtx(ctx, time.Duration(300+rand.IntN(500))*time.Millisecond)
	}

	// Phase 4: Fill remaining time with gentle random moves.
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return eventCount, ctx.Err()
		default:
		}

		if cursor != nil {
			startX, startY := cursor.Position()
			// Small drift within viewport.
			targetX := startX + (rand.Float64()-0.5)*200
			targetY := startY + (rand.Float64()-0.5)*150
			// Clamp to viewport.
			targetX = max(50, min(1400, targetX))
			targetY = max(50, min(850, targetY))

			steps := 3 + rand.IntN(4)
			path := humanize.BezierPath(startX, startY, targetX, targetY, steps)
			for _, p := range path {
				_ = proto.InputDispatchMouseEvent{
					Type: proto.InputDispatchMouseEventTypeMouseMoved,
					X:    p.X, Y: p.Y,
				}.Call(page)
				cursor.MoveTo(p.X, p.Y)
				eventCount++
			}
		}

		sleepCtx(ctx, time.Duration(500+rand.IntN(1000))*time.Millisecond)
	}

	return eventCount, nil
}

type hoverTarget struct {
	x, y float64
}

// findHoverTargets extracts clickable element positions from the page.
// Uses DOM.getDocument + querySelectorAll for links/buttons.
func findHoverTargets(page *rod.Page) []hoverTarget {
	var targets []hoverTarget

	// Get clickable elements.
	els, err := page.Elements("a[href], button, [role=button], [role=link]")
	if err != nil {
		return targets
	}

	for _, el := range els {
		box, err := el.Shape()
		if err != nil || len(box.Quads) == 0 {
			continue
		}
		// Use center of first quad.
		q := box.Quads[0]
		if len(q) < 8 {
			continue
		}
		cx := (q[0] + q[2] + q[4] + q[6]) / 4
		cy := (q[1] + q[3] + q[5] + q[7]) / 4
		// Only visible elements (in viewport).
		if cx > 0 && cx < 1500 && cy > 0 && cy < 2000 {
			targets = append(targets, hoverTarget{x: cx, y: cy})
		}
	}

	// Shuffle for natural order.
	rand.Shuffle(len(targets), func(i, j int) {
		targets[i], targets[j] = targets[j], targets[i]
	})

	return targets
}
