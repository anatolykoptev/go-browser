package humanize

import (
	"math"
	"math/rand/v2"
)

// gaussJitter returns a standard normal sample using the Box-Muller transform.
// It is the shared gaussian primitive used by mouse and keyboard humanizers.
func gaussJitter() float64 {
	u1 := rand.Float64()
	u2 := rand.Float64()
	// Avoid log(0).
	if u1 == 0 {
		u1 = 1e-10
	}
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

// clamp restricts v to the closed interval [lo, hi].
func clamp(v, lo, hi float64) float64 {
	return max(lo, min(hi, v))
}
