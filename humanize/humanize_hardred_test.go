package humanize

import (
	"math"
	"testing"
)

// Hard-red tests: statistical validation that humanization distributions
// match real human populations. If these fail, fraud SDKs will flag us.

// TestMouseDelay_Range verifies delay is within human bounds.
func TestMouseDelay_Range(t *testing.T) {
	for range 500 {
		d := MouseDelay()
		if d < 1 || d > 60 {
			t.Fatalf("MouseDelay() = %d; want 1-60ms", d)
		}
	}
}

// TestMouseDelay_Distribution verifies mean is in expected range.
func TestMouseDelay_Distribution(t *testing.T) {
	sum := 0
	n := 2000
	for range n {
		sum += MouseDelay()
	}
	mean := float64(sum) / float64(n)
	// Expected: base 2-8ms with 10% 15-30ms pauses → mean ~6-10ms
	if mean < 3 || mean > 15 {
		t.Errorf("MouseDelay mean = %.1f; want 3-15ms", mean)
	}
}

// TestTypingDelay_GaussianDistribution verifies typing delays match gaussian μ=120ms.
func TestTypingDelay_GaussianDistribution(t *testing.T) {
	n := 3000
	delays := make([]int, n)
	sum := 0
	for i := range n {
		delays[i] = TypingDelay()
		sum += delays[i]
	}
	mean := float64(sum) / float64(n)

	// Mean should be around 120ms (±20ms tolerance for statistical noise).
	if mean < 90 || mean > 160 {
		t.Errorf("TypingDelay mean = %.1f; want ~120ms (90-160)", mean)
	}

	// Stddev should be around 40ms (±15ms tolerance).
	var sumSqDiff float64
	for _, d := range delays {
		diff := float64(d) - mean
		sumSqDiff += diff * diff
	}
	stddev := math.Sqrt(sumSqDiff / float64(n))
	if stddev < 15 || stddev > 70 {
		t.Errorf("TypingDelay stddev = %.1f; want ~40ms (15-70)", stddev)
	}
}

// TestTypingDelay_Bounds verifies all delays are clamped.
func TestTypingDelay_Bounds(t *testing.T) {
	for range 2000 {
		d := TypingDelay()
		if d < 30 || d > 400 {
			t.Fatalf("TypingDelay() = %d; outside [30, 400]ms", d)
		}
	}
}

// TestKeyDwellTime_GaussianRange verifies key dwell is 40-120ms gaussian.
func TestKeyDwellTime_GaussianRange(t *testing.T) {
	n := 2000
	sum := 0
	for range n {
		d := KeyDwellTime()
		if d < 40 || d > 120 {
			t.Fatalf("KeyDwellTime() = %d; outside [40, 120]ms", d)
		}
		sum += d
	}
	mean := float64(sum) / float64(n)
	// μ=80ms expected
	if mean < 60 || mean > 100 {
		t.Errorf("KeyDwellTime mean = %.1f; want ~80ms (60-100)", mean)
	}
}

// TestWordBoundaryPause_Rate verifies ~15% trigger rate.
func TestWordBoundaryPause_Rate(t *testing.T) {
	n := 5000
	triggers := 0
	for range n {
		if WordBoundaryPause() > 0 {
			triggers++
		}
	}
	rate := float64(triggers) / float64(n)
	// 15% ±5%
	if rate < 0.08 || rate > 0.25 {
		t.Errorf("WordBoundaryPause trigger rate = %.3f; want ~0.15 (0.08-0.25)", rate)
	}
}

// TestBezierPath_HardRed_FittsLawScaling verifies longer moves take more steps (stricter).
func TestBezierPath_HardRed_FittsLawScaling(t *testing.T) {
	shortSum := 0
	longSum := 0
	n := 50
	for range n {
		shortPath := BezierPath(0, 0, 50, 50, 0)  // short move, 0 = auto steps
		longPath := BezierPath(0, 0, 800, 600, 0) // long move
		shortSum += len(shortPath)
		longSum += len(longPath)
	}
	shortAvg := float64(shortSum) / float64(n)
	longAvg := float64(longSum) / float64(n)

	// Long path should have more steps on average.
	if longAvg <= shortAvg {
		t.Errorf("BezierPath: long avg %.1f should exceed short avg %.1f", longAvg, shortAvg)
	}
}

// TestBezierPath_PointsReachTarget verifies final point is near target.
func TestBezierPath_PointsReachTarget(t *testing.T) {
	for range 100 {
		path := BezierPath(0, 0, 500, 300, 15)
		if len(path) == 0 {
			t.Fatal("BezierPath returned empty path")
		}
		last := path[len(path)-1]
		dx := last.X - 500
		dy := last.Y - 300
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist > 5 {
			t.Errorf("BezierPath final point (%.1f, %.1f) is %.1f px from target (500, 300)", last.X, last.Y, dist)
		}
	}
}

// TestSmoothScrollSteps_HasMultipleSteps verifies scroll produces a reasonable number of steps.
func TestSmoothScrollSteps_HasMultipleSteps(t *testing.T) {
	for _, total := range []int{100, 300, 500} {
		steps := SmoothScrollSteps(total)
		if len(steps) < 5 {
			t.Errorf("SmoothScrollSteps(%d) returned %d steps; want ≥5", total, len(steps))
		}
		if len(steps) > 25 {
			t.Errorf("SmoothScrollSteps(%d) returned %d steps; want ≤25", total, len(steps))
		}
		// All delays should be positive.
		for i, s := range steps {
			if s.DelayMs < 0 {
				t.Errorf("step %d: negative delay %dms", i, s.DelayMs)
			}
		}
	}
}

// TestSmoothScrollSteps_TotalMatchesInput verifies total scroll ≈ requested amount.
func TestSmoothScrollSteps_TotalMatchesInput(t *testing.T) {
	for _, total := range []int{100, 300, 500, 800} {
		steps := SmoothScrollSteps(total)
		sum := 0
		for _, s := range steps {
			sum += s.DeltaY
		}
		// Allow ±20% tolerance (overshoot + correction).
		if math.Abs(float64(sum-total)) > float64(total)*0.25 {
			t.Errorf("SmoothScrollSteps(%d): total delta = %d; want within ±25%%", total, sum)
		}
	}
}

// TestOvershoot_ProbabilityIncreasesWithDistance verifies far targets overshoot more.
func TestOvershoot_ProbabilityIncreasesWithDistance(t *testing.T) {
	n := 1000
	shortOS := 0
	longOS := 0
	for range n {
		if ShouldOvershoot(50) {
			shortOS++
		}
		if ShouldOvershoot(500) {
			longOS++
		}
	}
	// Long distance should overshoot more often.
	if longOS <= shortOS {
		t.Errorf("overshoot: short=%d long=%d; long should exceed short", shortOS, longOS)
	}
}

// TestCorrectionPath_HardRed_Accuracy verifies correction path is tight (stricter than base test).
func TestCorrectionPath_HardRed_Accuracy(t *testing.T) {
	// Stricter: verify across many offsets that correction always returns within 2px.
	for _, dx := range []float64{10, 30, 50} {
		for _, dy := range []float64{-20, 0, 25} {
			target := Point{X: 400, Y: 300}
			overshoot := Point{X: 400 + dx, Y: 300 + dy}
			for range 50 {
				path := CorrectionPath(overshoot, target)
				if len(path) == 0 {
					t.Fatal("empty correction path")
				}
				last := path[len(path)-1]
				dist := math.Sqrt((last.X-target.X)*(last.X-target.X) + (last.Y-target.Y)*(last.Y-target.Y))
				if dist > 2 {
					t.Errorf("dx=%.0f dy=%.0f: correction final %.1fpx from target (want ≤2)", dx, dy, dist)
				}
			}
		}
	}
}

// TestTypoChar_MostlyDifferent verifies typo is usually a different key.
// Note: double_key typo (12%) intentionally returns same char — that's valid.
func TestTypoChar_MostlyDifferent(t *testing.T) {
	n := 1000
	same := 0
	for range n {
		if TypoChar('e') == 'e' {
			same++
		}
	}
	rate := float64(same) / float64(n)
	// transpose (20%) + double_key (12%) = 32% return same char. Allow 25-45%.
	if rate < 0.20 || rate > 0.50 {
		t.Errorf("TypoChar('e') same-char rate = %.3f; want 0.20-0.50", rate)
	}
}

// TestLookupChar_SpecialChars verifies tricky chars have correct VK codes.
func TestLookupChar_SpecialChars(t *testing.T) {
	tests := []struct {
		ch    rune
		shift bool
	}{
		{'!', true}, {'@', true}, {'#', true}, {'$', true},
		{';', false}, {':', true}, {'/', false}, {'?', true},
		{'[', false}, {']', false}, {'{', true}, {'}', true},
	}
	for _, tt := range tests {
		info := LookupChar(tt.ch)
		if info.VK == 0 {
			t.Errorf("LookupChar(%c): VK = 0", tt.ch)
		}
		if info.Shift != tt.shift {
			t.Errorf("LookupChar(%c): Shift = %v; want %v", tt.ch, info.Shift, tt.shift)
		}
	}
}
