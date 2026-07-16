package humanize_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
)

func TestIdleDrift_Bounds(t *testing.T) {
	cursor := humanize.NewCursor(500, 400)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// #59: Use atomic counter — driftFunc runs in a different goroutine.
	var calls atomic.Int32
	driftFunc := func(x, y float64) error {
		calls.Add(1)
		return nil
	}

	stop := humanize.StartIdleDrift(ctx, cursor, driftFunc)
	time.Sleep(1200 * time.Millisecond)
	stop()

	if calls.Load() == 0 {
		t.Error("expected at least 1 drift call")
	}

	x, y := cursor.Position()
	dx := x - 500
	dy := y - 400
	if dx > 15 || dx < -15 || dy > 15 || dy < -15 {
		t.Errorf("cursor drifted too far: (%v,%v) from (500,400)", x, y)
	}
}
