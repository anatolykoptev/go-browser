package humanize

import "math/rand/v2"

// TypoEvent describes a single keystroke emitted during typo correction.
type TypoEvent struct {
	Char        rune // character to type (0 for Backspace)
	IsBackspace bool
	DelayMs     int // delay before this event (ms)
	DwellMs     int // key hold time (ms)
}

const (
	typoProbability = 0.03 // 3% chance per character

	// TypoChar variant weights (must sum to 100).
	typoWeightAdjacent  = 55
	typoWeightTranspose = 20
	typoWeightDouble    = 12
	typoWeightSkip      = 8
	typoWeightMissSpace = 5
	typoWeightTotal     = 100

	// Realization pause before starting correction (100-250ms).
	typoRealizePauseMin = 100
	typoRealizePauseMax = 150 // added to min: [100, 250]

	// Backspace dwell (60-100ms).
	typoBackspaceDwellMin = 60
	typoBackspaceDwellMax = 40 // added to min: [60, 100]

	// Pause after Backspace before re-typing (30-80ms).
	typoPostBackspacePauseMin = 30
	typoPostBackspacePauseMax = 50 // added to min: [30, 80]

	// Correct char dwell — reuse normal key dwell range.
	typoCorrectDwellMin = keyDwellMin
	typoCorrectDwellMax = keyDwellMax
)

// qwertyAdjacent maps each key to its QWERTY-layout neighbors.
// Only printable ASCII keys commonly typed in form fields are included.
var qwertyAdjacent = map[rune][]rune{
	'q': {'w', 'a', 's'},
	'w': {'q', 'e', 'a', 's', 'd'},
	'e': {'w', 'r', 's', 'd', 'f'},
	'r': {'e', 't', 'd', 'f', 'g'},
	't': {'r', 'y', 'f', 'g', 'h'},
	'y': {'t', 'u', 'g', 'h', 'j'},
	'u': {'y', 'i', 'h', 'j', 'k'},
	'i': {'u', 'o', 'j', 'k', 'l'},
	'o': {'i', 'p', 'k', 'l'},
	'p': {'o', 'l'},
	'a': {'q', 'w', 's', 'z'},
	's': {'a', 'w', 'e', 'd', 'z', 'x'},
	'd': {'s', 'e', 'r', 'f', 'x', 'c'},
	'f': {'d', 'r', 't', 'g', 'c', 'v'},
	'g': {'f', 't', 'y', 'h', 'v', 'b'},
	'h': {'g', 'y', 'u', 'j', 'b', 'n'},
	'j': {'h', 'u', 'i', 'k', 'n', 'm'},
	'k': {'j', 'i', 'o', 'l', 'm'},
	'l': {'k', 'o', 'p'},
	'z': {'a', 's', 'x'},
	'x': {'z', 's', 'd', 'c'},
	'c': {'x', 'd', 'f', 'v'},
	'v': {'c', 'f', 'g', 'b'},
	'b': {'v', 'g', 'h', 'n'},
	'n': {'b', 'h', 'j', 'm'},
	'm': {'n', 'j', 'k'},
	' ': {'c', 'v', 'b', 'n', 'm'},
}

// ShouldTypo returns true with 3% probability.
func ShouldTypo() bool {
	return rand.Float64() < typoProbability
}

// TypoChar returns a plausible wrong character for the given correct rune.
// Weights: adjacent_key 55%, transpose 20%, double_key 12%, skip 8%, missed_space 5%.
// Returns 0 for skip variant (caller should omit the character entirely).
func TypoChar(correct rune) rune {
	roll := rand.IntN(typoWeightTotal)
	switch {
	case roll < typoWeightAdjacent:
		// Adjacent key — pick a random QWERTY neighbor.
		neighbors := qwertyAdjacent[correct]
		if len(neighbors) == 0 {
			return correct + 1 // fallback for unmapped keys
		}
		return neighbors[rand.IntN(len(neighbors))]

	case roll < typoWeightAdjacent+typoWeightTranspose:
		// Transpose: swap with previous (caller handles); return a space as
		// a lightweight signal. Actual transposition is done in TypoCorrection.
		return correct

	case roll < typoWeightAdjacent+typoWeightTranspose+typoWeightDouble:
		// Double key: same char typed twice.
		return correct

	case roll < typoWeightAdjacent+typoWeightTranspose+typoWeightDouble+typoWeightSkip:
		// Skip: no char emitted — signal with 0.
		return 0

	default:
		// Missed space: insert extra space before the char.
		return ' '
	}
}

// TypoCorrection returns the sequence of TypoEvents to correct a single typo:
// realization pause → Backspace (with dwell) → post-backspace pause → correct char.
func TypoCorrection(correctChar rune) []TypoEvent {
	realizePause := typoRealizePauseMin + rand.IntN(typoRealizePauseMax+1)
	backspaceDwell := typoBackspaceDwellMin + rand.IntN(typoBackspaceDwellMax+1)
	postPause := typoPostBackspacePauseMin + rand.IntN(typoPostBackspacePauseMax+1)
	correctDwell := typoCorrectDwellMin + rand.IntN(typoCorrectDwellMax-typoCorrectDwellMin+1)

	return []TypoEvent{
		{
			Char:        0,
			IsBackspace: true,
			DelayMs:     realizePause,
			DwellMs:     backspaceDwell,
		},
		{
			Char:        correctChar,
			IsBackspace: false,
			DelayMs:     postPause,
			DwellMs:     correctDwell,
		},
	}
}
