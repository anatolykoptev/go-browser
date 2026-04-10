package humanize

import (
	"math"
	"math/rand/v2"
)

const (
	overshootProbFactor = 0.7
	overshootDistThresh = 300.0

	// Overshoot magnitude: 3–12% past target.
	overshootMinFrac = 0.03
	overshootMaxFrac = 0.12

	// Gaussian perpendicular spread during overshoot.
	overshootPerpSigmaFactor = 0.04

	// Correction path step count range.
	correctionMinSteps = 3
	correctionMaxSteps = 8

	// Correction bezier perpendicular spread: small (human re-targeting).
	correctionSpreadFactor = 0.08
)

// ShouldOvershoot returns true with probability 0.7 * min(1.0, distance/300).
// Shorter movements rarely overshoot; long movements do so ~70% of the time.
func ShouldOvershoot(distance float64) bool {
	prob := overshootProbFactor * math.Min(1.0, distance/overshootDistThresh)
	return rand.Float64() < prob
}

// OvershootPoint returns a point 3–12% past the target along the approach vector,
// with a small gaussian perpendicular offset to simulate imprecise stopping.
// The approach vector is from (0,0) direction implied by distance and target position,
// so we accept the full movement vector via fromX/fromY → targetX/targetY.
func OvershootPoint(fromX, fromY, targetX, targetY float64) Point {
	dx := targetX - fromX
	dy := targetY - fromY
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist == 0 {
		return Point{targetX, targetY}
	}

	// Unit vector along approach.
	ux, uy := dx/dist, dy/dist

	// Perpendicular unit vector.
	px, py := -uy, ux

	// Overshoot magnitude: random fraction of total distance.
	frac := overshootMinFrac + rand.Float64()*(overshootMaxFrac-overshootMinFrac)
	overshootDist := dist * frac

	// Small gaussian perpendicular jitter.
	perpOffset := gaussJitter() * dist * overshootPerpSigmaFactor

	return Point{
		X: targetX + ux*overshootDist + px*perpOffset,
		Y: targetY + uy*overshootDist + py*perpOffset,
	}
}

// CorrectionPath returns a short human-like correction path from the overshoot
// point back toward the target. Uses a gentle reverse Bezier with 3–8 steps
// and small perpendicular spread (simulating fine motor correction).
// The final point is the exact target.
func CorrectionPath(from, to Point) []Point {
	steps := correctionMinSteps + rand.IntN(correctionMaxSteps-correctionMinSteps+1)

	dx := to.X - from.X
	dy := to.Y - from.Y
	dist := math.Sqrt(dx*dx + dy*dy)

	// Single interior control point with small perpendicular offset.
	var cpX, cpY float64
	if dist > 0 {
		ux, uy := dx/dist, dy/dist
		px, py := -uy, ux
		spread := gaussJitter() * dist * correctionSpreadFactor
		cpX = from.X + dx*0.5 + px*spread
		cpY = from.Y + dy*0.5 + py*spread
	} else {
		cpX, cpY = from.X, from.Y
	}

	// Sample quadratic bezier: from → controlPoint → to.
	controlPts := []Point{from, {cpX, cpY}, to}
	path := make([]Point, steps)
	for i := range steps {
		var t float64
		if steps > 1 {
			t = float64(i) / float64(steps-1)
		}
		path[i] = deCasteljau(controlPts, t)
	}
	return path
}
