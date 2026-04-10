package humanize

import (
	"testing"
)

func TestSmoothScrollSteps_StepCount(t *testing.T) {
	for range 50 {
		steps := SmoothScrollSteps(300)
		// Base steps are 8-15; overshoot can add up to 4 more.
		if len(steps) < scrollMinSteps || len(steps) > scrollMaxSteps+4 {
			t.Errorf("got %d steps, want %d–%d", len(steps), scrollMinSteps, scrollMaxSteps+4)
		}
	}
}

func TestSmoothScrollSteps_DecayingDeltas(t *testing.T) {
	const runs = 100
	decayCount := 0
	for range runs {
		steps := SmoothScrollSteps(500)
		// Compare first vs last base step (exclude overshoot correction steps).
		base := steps
		if len(base) > scrollMaxSteps {
			base = steps[:scrollMaxSteps]
		}
		if len(base) < 2 {
			continue
		}
		first := abs(base[0].DeltaY)
		last := abs(base[len(base)-1].DeltaY)
		if first > last {
			decayCount++
		}
	}
	// Bezier easing should produce decaying deltas in most runs.
	// easeInOutCubic front-loads motion so first > last for most step counts.
	if decayCount < runs*65/100 {
		t.Errorf("decaying deltas only in %d/%d runs, want ≥65%%", decayCount, runs)
	}
}

func TestSmoothScrollSteps_TotalSumMatchesInput(t *testing.T) {
	totals := []int{100, 300, -200, 500, -50}
	for _, total := range totals {
		t.Run("", func(t *testing.T) {
			steps := SmoothScrollSteps(total)
			sum := 0
			for _, s := range steps {
				sum += s.DeltaY
			}
			// Allow 10% deviation for overshoot cases.
			tolerance := abs(total) / 10
			if tolerance < 5 {
				tolerance = 5
			}
			diff := abs(sum - total)
			if diff > tolerance {
				t.Errorf("sum=%d total=%d diff=%d exceeds tolerance %d", sum, total, diff, tolerance)
			}
		})
	}
}

func TestSmoothScrollSteps_MicroPauses(t *testing.T) {
	const runs = 200
	found := false
	for range runs {
		steps := SmoothScrollSteps(400)
		for _, s := range steps {
			if s.DelayMs >= scrollPauseMin {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("no micro-pause (DelayMs >= %d) found in %d runs", scrollPauseMin, runs)
	}
}

func TestSmoothScrollSteps_DeltaXJitter(t *testing.T) {
	const runs = 100
	found := false
	for range runs {
		steps := SmoothScrollSteps(300)
		for _, s := range steps {
			if s.DeltaX != 0 {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("no DeltaX jitter found in %d runs", runs)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
