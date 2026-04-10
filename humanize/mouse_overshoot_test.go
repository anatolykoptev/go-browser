package humanize

import (
	"math"
	"testing"
)

// TestShouldOvershoot_Probability verifies that at distance=500px, ~70% of
// calls return true (tolerance ±10%).
func TestShouldOvershoot_Probability(t *testing.T) {
	const (
		trials   = 1000
		distance = 500.0
		wantLow  = 0.60
		wantHigh = 0.80
	)

	var trueCount int
	for range trials {
		if ShouldOvershoot(distance) {
			trueCount++
		}
	}

	rate := float64(trueCount) / trials
	if rate < wantLow || rate > wantHigh {
		t.Errorf("ShouldOvershoot(distance=500) rate=%.2f, want [%.2f, %.2f]",
			rate, wantLow, wantHigh)
	}
}

// TestShouldOvershoot_NeverAtZero verifies that distance=0 never overshoots.
func TestShouldOvershoot_NeverAtZero(t *testing.T) {
	for range 200 {
		if ShouldOvershoot(0) {
			t.Error("ShouldOvershoot(0) returned true, want always false")
			return
		}
	}
}

// TestOvershootPoint_Distance verifies the overshoot lands 3–12% past the target
// measured along the movement vector.
func TestOvershootPoint_Distance(t *testing.T) {
	const (
		fromX, fromY     = 0.0, 0.0
		targetX, targetY = 400.0, 300.0
	)
	totalDist := math.Sqrt((targetX-fromX)*(targetX-fromX) + (targetY-fromY)*(targetY-fromY))

	for trial := range 200 {
		pt := OvershootPoint(fromX, fromY, targetX, targetY)

		// Project pt onto the approach axis relative to target.
		dx := targetX - fromX
		dy := targetY - fromY
		ux, uy := dx/totalDist, dy/totalDist

		// Signed distance from target along approach axis.
		overX := pt.X - targetX
		overY := pt.Y - targetY
		alongAxis := overX*ux + overY*uy

		// Must be forward (positive = past target).
		if alongAxis < 0 {
			t.Errorf("trial %d: overshoot point is behind target (along=%.3f)", trial, alongAxis)
			continue
		}

		fracOver := alongAxis / totalDist
		if fracOver < overshootMinFrac-1e-9 || fracOver > overshootMaxFrac+1e-9 {
			t.Errorf("trial %d: overshoot fraction=%.4f, want [%.2f, %.2f]",
				trial, fracOver, overshootMinFrac, overshootMaxFrac)
		}
	}
}

// TestCorrectionPath_ReturnsToTarget verifies that the final point in the
// correction path is within 2px of the target.
func TestCorrectionPath_ReturnsToTarget(t *testing.T) {
	from := Point{410, 315}
	to := Point{400, 300}
	const tolerance = 2.0

	for trial := range 50 {
		path := CorrectionPath(from, to)
		if len(path) == 0 {
			t.Fatalf("trial %d: CorrectionPath returned empty slice", trial)
		}
		last := path[len(path)-1]
		dist := math.Sqrt((last.X-to.X)*(last.X-to.X) + (last.Y-to.Y)*(last.Y-to.Y))
		if dist > tolerance {
			t.Errorf("trial %d: final point (%.2f, %.2f) is %.2fpx from target (%.2f, %.2f), want ≤%.1f",
				trial, last.X, last.Y, dist, to.X, to.Y, tolerance)
		}
	}
}

// TestCorrectionPath_StepCount verifies the path has between 3 and 8 points.
func TestCorrectionPath_StepCount(t *testing.T) {
	from := Point{410, 315}
	to := Point{400, 300}

	for trial := range 200 {
		path := CorrectionPath(from, to)
		if len(path) < correctionMinSteps || len(path) > correctionMaxSteps {
			t.Errorf("trial %d: CorrectionPath len=%d, want [%d, %d]",
				trial, len(path), correctionMinSteps, correctionMaxSteps)
		}
	}
}
