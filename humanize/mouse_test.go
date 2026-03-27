package humanize

import (
	"math"
	"testing"
)

func TestBezierPath_Length(t *testing.T) {
	cases := []struct {
		steps  int
		minPts int
		maxPts int
	}{
		{20, 10, 50},
		{5, 5, 10},
		{1, minSteps, minSteps}, // enforces minimum
	}
	for _, tc := range cases {
		pts := BezierPath(0, 0, 100, 100, tc.steps)
		if len(pts) < tc.minPts || len(pts) > tc.maxPts {
			t.Errorf("BezierPath(steps=%d): got %d points, want [%d, %d]",
				tc.steps, len(pts), tc.minPts, tc.maxPts)
		}
	}
}

func TestBezierPath_StartsAtOrigin(t *testing.T) {
	const tolerance = 1.0
	startX, startY := 123.0, 456.0
	pts := BezierPath(startX, startY, 800, 600, 20)
	if len(pts) == 0 {
		t.Fatal("BezierPath returned empty slice")
	}
	first := pts[0]
	if math.Abs(first.X-startX) > tolerance || math.Abs(first.Y-startY) > tolerance {
		t.Errorf("first point (%.2f, %.2f) not near start (%.2f, %.2f) within %.1f",
			first.X, first.Y, startX, startY, tolerance)
	}
}

func TestBezierPath_EndsAtTarget(t *testing.T) {
	const tolerance = 5.0
	endX, endY := 800.0, 600.0
	pts := BezierPath(0, 0, endX, endY, 20)
	if len(pts) == 0 {
		t.Fatal("BezierPath returned empty slice")
	}
	last := pts[len(pts)-1]
	if math.Abs(last.X-endX) > tolerance || math.Abs(last.Y-endY) > tolerance {
		t.Errorf("last point (%.2f, %.2f) not near end (%.2f, %.2f) within %.1f",
			last.X, last.Y, endX, endY, tolerance)
	}
}
