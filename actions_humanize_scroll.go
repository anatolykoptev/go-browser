package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// doScrollHumanized performs a human-like scroll using 8–15 wheel CDP events
// with Bezier easing, momentum decay, and random micro-pauses.
func doScrollHumanized(ctx context.Context, page *rod.Page, deltaY int, cursor *humanize.Cursor) error {
	steps := humanize.SmoothScrollSteps(deltaY)
	cx, cy := cursor.Position()

	for _, s := range steps {
		ev := proto.InputDispatchMouseEvent{
			Type:   proto.InputDispatchMouseEventTypeMouseWheel,
			X:      cx,
			Y:      cy,
			DeltaX: float64(s.DeltaX),
			DeltaY: float64(s.DeltaY),
		}
		if err := ev.Call(page); err != nil {
			return fmt.Errorf("scroll step: %w", err)
		}
		sleepCtx(ctx, time.Duration(s.DelayMs)*time.Millisecond)
	}
	return nil
}
