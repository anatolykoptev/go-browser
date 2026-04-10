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
		{20, 20, 20},
		{5, 5, 5},
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

// TestBezierPath_MinimumJerk_DenseAtEnds verifies that the minimum-jerk profile
// produces smaller inter-point distances in the first and last 20% of steps
// compared to the middle 60%.
func TestBezierPath_MinimumJerk_DenseAtEnds(t *testing.T) {
	const steps = 100
	pts := BezierPath(0, 0, 1000, 0, steps) // horizontal, no jitter dominance

	dist := func(a, b Point) float64 {
		dx, dy := b.X-a.X, b.Y-a.Y
		return math.Sqrt(dx*dx + dy*dy)
	}

	sumZone := func(from, to int) float64 {
		var s float64
		for i := from; i < to && i < len(pts)-1; i++ {
			s += dist(pts[i], pts[i+1])
		}
		return s / float64(to-from)
	}

	endBand := steps / 5      // 20% = 20 steps
	midStart := steps * 2 / 5 // 40%
	midEnd := steps * 3 / 5   // 60%

	avgStart := sumZone(0, endBand)
	avgMid := sumZone(midStart, midEnd)
	avgEnd := sumZone(steps-endBand, steps-1)

	if avgMid <= avgStart {
		t.Errorf("middle avg distance (%.2f) should exceed start band (%.2f) — minimum-jerk violated", avgMid, avgStart)
	}
	if avgMid <= avgEnd {
		t.Errorf("middle avg distance (%.2f) should exceed end band (%.2f) — minimum-jerk violated", avgMid, avgEnd)
	}
}

// TestBezierPath_FittsLaw_ScalesWithDistance verifies that a longer movement
// produces more steps when steps=0 (Fitts' law auto-compute).
func TestBezierPath_FittsLaw_ScalesWithDistance(t *testing.T) {
	shortPts := BezierPath(0, 0, 100, 0, 0)
	longPts := BezierPath(0, 0, 1000, 0, 0)

	if len(longPts) <= len(shortPts) {
		t.Errorf("1000px move (%d steps) should have more steps than 100px move (%d steps)",
			len(longPts), len(shortPts))
	}
}

// TestBezierPath_ControlPoints_3to5 checks that the generated path has no
// abrupt angle changes > 90° between consecutive segments, ensuring smooth curves.
func TestBezierPath_ControlPoints_3to5(t *testing.T) {
	const steps = 50
	// Run multiple times since control points are random.
	for trial := range 10 {
		pts := BezierPath(0, 0, 500, 300, steps)
		for i := 1; i < len(pts)-1; i++ {
			ax := pts[i].X - pts[i-1].X
			ay := pts[i].Y - pts[i-1].Y
			bx := pts[i+1].X - pts[i].X
			by := pts[i+1].Y - pts[i].Y
			magA := math.Sqrt(ax*ax + ay*ay)
			magB := math.Sqrt(bx*bx + by*by)
			if magA < 1e-9 || magB < 1e-9 {
				continue
			}
			cosTheta := (ax*bx + ay*by) / (magA * magB)
			cosTheta = max(-1.0, min(1.0, cosTheta))
			angle := math.Acos(cosTheta) * 180 / math.Pi
			if angle > 90 {
				t.Errorf("trial %d: abrupt angle change %.1f° at step %d", trial, angle, i)
				break
			}
		}
	}
}

// TestDeCasteljau_LinearInterpolation verifies that with exactly 2 points,
// deCasteljau produces the expected linear interpolation.
func TestDeCasteljau_LinearInterpolation(t *testing.T) {
	p0 := Point{0, 0}
	p1 := Point{100, 200}
	pts := []Point{p0, p1}

	cases := []struct{ t, wantX, wantY float64 }{
		{0.0, 0, 0},
		{0.5, 50, 100},
		{1.0, 100, 200},
		{0.25, 25, 50},
	}
	for _, tc := range cases {
		got := deCasteljau(pts, tc.t)
		if math.Abs(got.X-tc.wantX) > 1e-9 || math.Abs(got.Y-tc.wantY) > 1e-9 {
			t.Errorf("deCasteljau(t=%.2f): got (%.2f, %.2f), want (%.2f, %.2f)",
				tc.t, got.X, got.Y, tc.wantX, tc.wantY)
		}
	}
}

// TestMouseDelayForStep_SlowAtEnds verifies that delay at step 0 is greater
// than delay at the midpoint step.
func TestMouseDelayForStep_SlowAtEnds(t *testing.T) {
	const totalSteps = 100
	// Run multiple samples since there's randomness in the slow band.
	startSum, midSum := 0, 0
	const trials = 50
	for range trials {
		startSum += MouseDelayForStep(0, totalSteps)
		midSum += MouseDelayForStep(totalSteps/2, totalSteps)
	}
	avgStart := startSum / trials
	avgMid := midSum / trials
	if avgStart <= avgMid {
		t.Errorf("avg delay at step 0 (%dms) should exceed avg delay at midpoint (%dms)", avgStart, avgMid)
	}
}
