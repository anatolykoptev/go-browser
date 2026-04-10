package humanize

import (
	"math"
	"math/rand/v2"
)

// Point is a 2D coordinate.
type Point struct{ X, Y float64 }

const (
	minSteps      = 5
	mouseBaseMin  = 2
	mouseBaseMax  = 8
	mousePauseLow = 15
	mouseBaseHigh = 30
	pauseChance   = 0.10

	// Fitts' law constants (MT = a + b*log2(D/W+1), seconds).
	fittsA       = 0.070
	fittsB       = 0.150
	fittsWidth   = 40.0 // default target width (px), typical button
	stepInterval = 12.0 // ms per step

	// Control point count range.
	minControlPoints = 3
	extraControlPts  = 3 // rand in [0, extraControlPts) → total in [3,5]

	// Delay bands (ms).
	delayFastMs  = 8
	delaySlowMin = 20
	delaySlowMax = 30
	// Fraction of path considered "near start/end" for slow delay.
	endZone = 0.20

	// DwellDelay constants (T3 behavioral biometric).
	dwellBaseMs       = 50
	dwellTargetFactor = 100.0
	dwellTargetMin    = 20.0
	dwellJitterMax    = 100
	dwellMax          = 300
	dwellHesitateMin  = 300
	dwellHesitateMax  = 200
	dwellHesitateProb = 0.05
	dwellMicroSigma   = 1.0
)

// fittsSteps computes the number of movement steps for a given pixel
// distance using Fitts' law: MT = a + b*log2(D/W+1) seconds.
func fittsSteps(dist float64) int {
	mt := fittsA + fittsB*math.Log2(dist/fittsWidth+1)
	steps := int(mt * 1000 / stepInterval)
	return max(minSteps, steps)
}

// BezierPath generates a human-like mouse path using biomechanics-informed
// Bezier curves. Key properties:
//   - 3–5 random control points with perpendicular gaussian spread
//   - Minimum-jerk velocity profile (Flash & Hogan 1985): dense points at
//     start/end (slow) and sparse in middle (fast)
//   - If steps <= 0, step count is derived from Fitts' law (MT formula)
//
// Signature is backward-compatible: existing callers that pass steps > 0
// keep their count but gain minimum-jerk timing.
func BezierPath(startX, startY, endX, endY float64, steps int) []Point {
	dx := endX - startX
	dy := endY - startY
	dist := math.Sqrt(dx*dx + dy*dy)

	if steps <= 0 {
		steps = fittsSteps(dist)
	} else if steps < minSteps {
		steps = minSteps
	}

	start := Point{startX, startY}
	end := Point{endX, endY}

	// Build full control point list: start + N interior points + end.
	n := minControlPoints + rand.IntN(extraControlPts) // [3, 5]
	interior := buildControlPoints(start, end, n, gaussJitter)
	allPts := make([]Point, 0, n+2)
	allPts = append(allPts, start)
	allPts = append(allPts, interior...)
	allPts = append(allPts, end)

	// Sample the curve at minimum-jerk time positions.
	points := make([]Point, steps)
	for i := range steps {
		var tUniform float64
		if steps > 1 {
			tUniform = float64(i) / float64(steps-1)
		}
		t := minimumJerk(tUniform)
		points[i] = deCasteljau(allPts, t)
	}
	return points
}

// MouseDelay returns a human-like delay between mouse move steps in milliseconds.
// Base delay is 2–8ms with a 10% chance of a 15–30ms pause.
// Use MouseDelayForStep for position-aware variation.
func MouseDelay() int {
	base := mouseBaseMin + rand.IntN(mouseBaseMax-mouseBaseMin+1)
	if rand.Float64() < pauseChance {
		return mousePauseLow + rand.IntN(mouseBaseHigh-mousePauseLow+1)
	}
	return base
}

// DwellDelay returns a pre-click pause duration and micro-movements to simulate
// visual acquisition delay (T3 behavioral biometric tracked by TMX).
// targetWidth: width of the click target in pixels — smaller targets need longer dwell.
// delayMs is in [50, 500]; microMoves contains 1-3 sub-pixel settling offsets.
func DwellDelay(targetWidth float64) (delayMs int, microMoves []Point) {
	// Inverse relationship: small targets need more time for visual acquisition.
	base := dwellBaseMs + int(dwellTargetFactor*(1/math.Max(targetWidth, dwellTargetMin))*dwellTargetMin)
	jitter := rand.IntN(dwellJitterMax)
	delayMs = min(dwellMax, base+jitter)

	// 5% chance of hesitation dwell (300-500ms): user second-guessing the target.
	if rand.Float64() < dwellHesitateProb {
		delayMs = dwellHesitateMin + rand.IntN(dwellHesitateMax)
	}

	// 1-3 micro-movements during dwell: hand settling on the target.
	n := 1 + rand.IntN(3)
	microMoves = make([]Point, n)
	for i := range n {
		microMoves[i] = Point{
			X: gaussJitter() * dwellMicroSigma,
			Y: gaussJitter() * dwellMicroSigma,
		}
	}
	return
}

// MouseDelayForStep returns a position-aware delay in milliseconds that mirrors
// the minimum-jerk velocity profile: slower at path endpoints (20–30ms) and
// faster in the middle (8ms). This supplements the spatial minimum-jerk curve
// with temporal variation for more realistic bot-evasion behavior.
func MouseDelayForStep(stepIndex, totalSteps int) int {
	if totalSteps <= 1 {
		return delaySlowMin
	}
	frac := float64(stepIndex) / float64(totalSteps-1)
	// Slow at start (frac < endZone) and end (frac > 1-endZone).
	if frac < endZone || frac > 1-endZone {
		return delaySlowMin + rand.IntN(delaySlowMax-delaySlowMin+1)
	}
	return delayFastMs
}
