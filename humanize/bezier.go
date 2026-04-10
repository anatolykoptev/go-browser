package humanize

import "math"

// minimumJerk maps a uniform t ∈ [0,1] to the minimum-jerk time profile
// (Flash & Hogan 1985). Points are dense at start and end (slow) and
// sparse in the middle (fast), producing a bell-shaped velocity profile
// that ML classifiers expect for human-like movement.
func minimumJerk(t float64) float64 {
	t2 := t * t
	t3 := t2 * t
	t4 := t3 * t
	t5 := t4 * t
	return 10*t3 - 15*t4 + 6*t5
}

// deCasteljau evaluates a Bezier curve of arbitrary degree at parameter t
// using De Casteljau's algorithm. Works for any number of control points.
func deCasteljau(points []Point, t float64) Point {
	pts := make([]Point, len(points))
	copy(pts, points)
	for len(pts) > 1 {
		next := make([]Point, len(pts)-1)
		for i := range next {
			u := 1 - t
			next[i] = Point{
				X: pts[i].X*u + pts[i+1].X*t,
				Y: pts[i].Y*u + pts[i+1].Y*t,
			}
		}
		pts = next
	}
	return pts[0]
}

// buildControlPoints generates n interior control points (n ∈ [3,5])
// distributed at evenly-spaced t values along start→end, each offset
// perpendicularly with gaussian spread sigma = dist * 0.15.
func buildControlPoints(start, end Point, n int, gaussJitter func() float64) []Point {
	dx := end.X - start.X
	dy := end.Y - start.Y
	dist := math.Sqrt(dx*dx + dy*dy)

	var perpX, perpY float64
	if dist > 0 {
		perpX, perpY = -dy/dist, dx/dist
	}

	const sigmaFactor = 0.15
	sigma := dist * sigmaFactor

	pts := make([]Point, n)
	for i := range n {
		// t positions evenly spaced: for n=3 → 0.25, 0.50, 0.75
		//                            for n=4 → 0.20, 0.40, 0.60, 0.80
		//                            for n=5 → 0.167, 0.333, 0.5, 0.667, 0.833
		tPos := float64(i+1) / float64(n+1)
		offset := gaussJitter() * sigma
		pts[i] = Point{
			X: start.X + dx*tPos + perpX*offset,
			Y: start.Y + dy*tPos + perpY*offset,
		}
	}
	return pts
}
