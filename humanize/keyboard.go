package humanize

import (
	"math/rand/v2"
)

const (
	typingDelayMin = 30
	typingDelayMax = 300

	// Gaussian TypingDelay parameters (μ=120ms, σ=40ms).
	typingMeanMs  = 120.0
	typingSigmaMs = 40.0

	// KeyDwellTime parameters (μ=80ms, σ=25ms, clamped [40, 120]).
	keyDwellMeanMs  = 80.0
	keyDwellSigmaMs = 25.0
	keyDwellMin     = 40
	keyDwellMax     = 120

	// WordBoundaryPause parameters.
	wordBoundaryProb   = 0.15
	wordBoundaryMin    = 300
	wordBoundaryJitter = 200
)

// TypingDelay returns a human-like inter-key delay in milliseconds.
// Gaussian μ=120ms σ=40ms, clamped to [30, 300].
func TypingDelay() int {
	z := gaussJitter()
	delay := typingMeanMs + z*typingSigmaMs
	return max(typingDelayMin, min(typingDelayMax, int(delay)))
}

// KeyDwellTime returns the duration in milliseconds a key should be held down
// (keyDown → keyUp). Gaussian μ=80ms σ=25ms, clamped to [40, 120].
// TMX tracks key_dwell_time as a primary behavioral biometric (T4).
func KeyDwellTime() int {
	z := gaussJitter()
	dwell := keyDwellMeanMs + z*keyDwellSigmaMs
	return max(keyDwellMin, min(keyDwellMax, int(dwell)))
}

// WordBoundaryPause returns an extra delay at word boundaries (spaces).
// 15% chance of 300-500ms pause simulating inter-word cognitive processing;
// returns 0 otherwise.
func WordBoundaryPause() int {
	if rand.Float64() < wordBoundaryProb {
		return wordBoundaryMin + rand.IntN(wordBoundaryJitter)
	}
	return 0
}

// TypingDelays generates per-character delays for a string.
// Returns one delay per Unicode code point.
func TypingDelays(text string) []int {
	runes := []rune(text)
	delays := make([]int, len(runes))
	for i := range runes {
		delays[i] = TypingDelay()
	}
	return delays
}
