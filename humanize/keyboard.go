package humanize

import "math/rand/v2"

const (
	typingBaseMin     = 50
	typingBaseMax     = 120
	typingPauseMin    = 50
	typingPauseMax    = 150
	typingPauseChance = 0.15
	typingDelayMin    = 30
	typingDelayMax    = 300
)

// TypingDelay returns a human-like delay for a single keystroke in milliseconds.
// Base is 50-120ms with a 15% chance of an additional 50-150ms thinking pause.
// Result is clamped to [30, 300].
func TypingDelay() int {
	delay := typingBaseMin + rand.IntN(typingBaseMax-typingBaseMin+1)
	if rand.Float64() < typingPauseChance {
		delay += typingPauseMin + rand.IntN(typingPauseMax-typingPauseMin+1)
	}
	if delay < typingDelayMin {
		return typingDelayMin
	}
	if delay > typingDelayMax {
		return typingDelayMax
	}
	return delay
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
