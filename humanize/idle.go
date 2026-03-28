package humanize

import (
	"context"
	"math/rand/v2"
	"time"
)

const (
	driftMinIntervalMs = 300
	driftMaxIntervalMs = 1000
	driftMaxPixels     = 3.0
	driftClamp         = 15.0
)

// DriftFunc dispatches a mouse move event at (x, y).
type DriftFunc func(x, y float64) error

// StartIdleDrift starts a goroutine that sends micro-movements at random intervals.
// Returns a stop function. Also stops when ctx is cancelled.
func StartIdleDrift(ctx context.Context, cursor *Cursor, dispatch DriftFunc) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		originX, originY := cursor.Position()
		for {
			interval := driftMinIntervalMs + rand.IntN(driftMaxIntervalMs-driftMinIntervalMs)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(interval) * time.Millisecond):
			}

			x, y := cursor.Position()
			dx := (rand.Float64()*2 - 1) * driftMaxPixels
			dy := (rand.Float64()*2 - 1) * driftMaxPixels

			newX := x + dx
			newY := y + dy

			if newX-originX > driftClamp {
				newX = originX + driftClamp
			} else if newX-originX < -driftClamp {
				newX = originX - driftClamp
			}
			if newY-originY > driftClamp {
				newY = originY + driftClamp
			} else if newY-originY < -driftClamp {
				newY = originY - driftClamp
			}

			if err := dispatch(newX, newY); err != nil {
				return
			}
			cursor.MoveTo(newX, newY)
		}
	}()

	return cancel
}
