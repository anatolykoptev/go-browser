package humanize

import (
	"math"
	"math/rand/v2"
)

// ScrollStep is a single wheel event in a humanized scroll sequence.
type ScrollStep struct {
	DeltaY  int // vertical scroll delta (pixels, positive = scroll down)
	DeltaX  int // horizontal jitter
	DelayMs int // delay after this step (milliseconds)
}

const (
	scrollMinSteps      = 8
	scrollMaxSteps      = 15
	scrollBaseDelayMin  = 12
	scrollBaseDelayMax  = 16
	scrollJitterRange   = 3 // ±3ms timing jitter
	scrollPauseChance   = 0.05
	scrollPauseMin      = 20
	scrollPauseMax      = 50
	scrollOvershootProb = 0.15
	scrollOvershootPct  = 0.06 // max overshoot as fraction of totalDelta
	scrollDeltaXClamp   = 3
	scrollDeltaXSigma   = 1.5
)

// SmoothScrollSteps generates 8–15 wheel events that simulate human scrolling.
// totalDelta: total pixels to scroll (positive = down, negative = up).
// Uses cubic Bezier easing for momentum decay with random micro-pauses.
func SmoothScrollSteps(totalDelta int) []ScrollStep {
	numSteps := scrollMinSteps + rand.IntN(scrollMaxSteps-scrollMinSteps+1)
	steps := make([]ScrollStep, 0, numSteps+3)

	// Compute per-step deltas from eased cumulative progress.
	prevProgress := 0.0
	remaining := totalDelta
	for i := range numSteps {
		t := float64(i+1) / float64(numSteps)
		progress := cubicBezierEasing(t)

		delta := int(math.Round((progress - prevProgress) * float64(totalDelta)))

		// Last step: assign all remaining to avoid drift from rounding.
		if i == numSteps-1 {
			delta = remaining
		}
		prevProgress = progress
		remaining -= delta

		steps = append(steps, ScrollStep{
			DeltaY:  delta,
			DeltaX:  clampedGaussX(),
			DelayMs: scrollDelay(),
		})
	}

	// Overshoot: add extra motion then correction steps.
	if rand.Float64() < scrollOvershootProb {
		overshoot := int(math.Round(float64(totalDelta) * (scrollOvershootPct + rand.Float64()*scrollOvershootPct)))
		if totalDelta < 0 {
			overshoot = -overshoot
		}
		steps = append(steps, ScrollStep{
			DeltaY:  overshoot,
			DeltaX:  clampedGaussX(),
			DelayMs: scrollDelay(),
		})
		// 2-3 correction steps back.
		corrSteps := 2 + rand.IntN(2)
		corrPer := -overshoot / corrSteps
		for range corrSteps {
			steps = append(steps, ScrollStep{
				DeltaY:  corrPer,
				DeltaX:  clampedGaussX(),
				DelayMs: scrollDelay(),
			})
		}
	}

	return steps
}

// cubicBezierEasing evaluates the eased progress at parameter t for the
// "ease-in-out" cubic bezier (0.645, 0.045, 0.355, 1.0).
// Produces fast early momentum that decays toward the end.
func cubicBezierEasing(t float64) float64 {
	// CSS cubic-bezier(0.645, 0.045, 0.355, 1.0) — "easeInOutCubic" variant.
	// Solved numerically: find s such that Bx(s) = t, then return By(s).
	const (
		p1x = 0.645
		p1y = 0.045
		p2x = 0.355
		p2y = 1.0
	)
	// Newton-Raphson to solve t_x(s) = t for s.
	s := t
	for range 8 {
		bx := bezierCoord(s, p1x, p2x)
		bxd := bezierCoordDeriv(s, p1x, p2x)
		if math.Abs(bxd) < 1e-9 {
			break
		}
		s -= (bx - t) / bxd
		s = math.Max(0, math.Min(1, s))
	}
	return bezierCoord(s, p1y, p2y)
}

// bezierCoord computes the cubic bezier coordinate for a given parameter s,
// with P0=0, P1=p1, P2=p2, P3=1.
func bezierCoord(s, p1, p2 float64) float64 {
	u := 1 - s
	return 3*u*u*s*p1 + 3*u*s*s*p2 + s*s*s
}

// bezierCoordDeriv is the derivative of bezierCoord with respect to s.
func bezierCoordDeriv(s, p1, p2 float64) float64 {
	u := 1 - s
	return 3*u*u*p1 + 6*u*s*(p2-p1) + 3*s*s*(1-p2)
}

// clampedGaussX returns a gaussian-jittered horizontal delta clamped to ±3.
func clampedGaussX() int {
	g := gaussJitter() * scrollDeltaXSigma
	v := int(math.Round(g))
	if v > scrollDeltaXClamp {
		return scrollDeltaXClamp
	}
	if v < -scrollDeltaXClamp {
		return -scrollDeltaXClamp
	}
	return v
}

// scrollDelay returns a human-like delay in milliseconds for a scroll step.
func scrollDelay() int {
	base := scrollBaseDelayMin + rand.IntN(scrollBaseDelayMax-scrollBaseDelayMin+1)
	jitter := rand.IntN(scrollJitterRange*2+1) - scrollJitterRange
	if rand.Float64() < scrollPauseChance {
		return scrollPauseMin + rand.IntN(scrollPauseMax-scrollPauseMin+1)
	}
	return base + jitter
}
