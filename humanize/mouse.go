package humanize

import (
	"math"
	"math/rand/v2"
)

// Point is a 2D coordinate.
type Point struct{ X, Y float64 }

const (
	minSteps      = 5
	jitterFactor  = 0.3
	mouseBaseMin  = 2
	mouseBaseMax  = 8
	mousePauseLow = 15
	mouseBaseHigh = 30
	pauseChance   = 0.10
)

// BezierPath generates a human-like mouse path using cubic Bezier curves.
// It uses 2 random control points with gaussian jitter (Box-Muller transform).
// Jitter is 30% of the distance between start and end.
func BezierPath(startX, startY, endX, endY float64, steps int) []Point {
	if steps < minSteps {
		steps = minSteps
	}

	dx := endX - startX
	dy := endY - startY
	dist := math.Sqrt(dx*dx+dy*dy) * jitterFactor

	// Box-Muller transform for gaussian jitter.
	gaussJitter := func() float64 {
		u1 := rand.Float64()
		u2 := rand.Float64()
		// Avoid log(0).
		if u1 == 0 {
			u1 = 1e-10
		}
		return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	}

	// Two random control points with gaussian offset.
	p0 := Point{startX, startY}
	p1 := Point{
		startX + dx*0.33 + gaussJitter()*dist,
		startY + dy*0.33 + gaussJitter()*dist,
	}
	p2 := Point{
		startX + dx*0.66 + gaussJitter()*dist,
		startY + dy*0.66 + gaussJitter()*dist,
	}
	p3 := Point{endX, endY}

	points := make([]Point, steps)
	for i := range steps {
		t := float64(i) / float64(steps-1)
		u := 1 - t
		// Cubic Bezier: u³·p0 + 3u²t·p1 + 3ut²·p2 + t³·p3
		coef0 := u * u * u
		coef1 := 3 * u * u * t
		coef2 := 3 * u * t * t
		coef3 := t * t * t
		points[i] = Point{
			X: coef0*p0.X + coef1*p1.X + coef2*p2.X + coef3*p3.X,
			Y: coef0*p0.Y + coef1*p1.Y + coef2*p2.Y + coef3*p3.Y,
		}
	}
	return points
}

// MouseDelay returns a human-like delay between mouse move steps in milliseconds.
// Base delay is 2-8ms with a 10% chance of a 15-30ms pause.
func MouseDelay() int {
	base := mouseBaseMin + rand.IntN(mouseBaseMax-mouseBaseMin+1)
	if rand.Float64() < pauseChance {
		return mousePauseLow + rand.IntN(mouseBaseHigh-mousePauseLow+1)
	}
	return base
}
