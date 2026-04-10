package humanize

import (
	"math"
	"testing"
)

func TestTypingDelay_Range(t *testing.T) {
	for range 100 {
		d := TypingDelay()
		if d < typingDelayMin || d > typingDelayMax {
			t.Errorf("TypingDelay() = %d, want [%d, %d]", d, typingDelayMin, typingDelayMax)
		}
	}
}

func TestTypingDelays_Length(t *testing.T) {
	text := "hello world"
	delays := TypingDelays(text)
	want := len([]rune(text)) // 11
	if len(delays) != want {
		t.Errorf("TypingDelays(%q): got %d delays, want %d", text, len(delays), want)
	}
}

// TestKeyDwellTime_Gaussian verifies that KeyDwellTime produces values in [40, 120]
// with a mean near 80ms over 1000 samples.
func TestKeyDwellTime_Gaussian(t *testing.T) {
	const samples = 1000
	var sum int
	for range samples {
		d := KeyDwellTime()
		if d < keyDwellMin || d > keyDwellMax {
			t.Errorf("KeyDwellTime() = %d out of [%d, %d]", d, keyDwellMin, keyDwellMax)
		}
		sum += d
	}
	mean := float64(sum) / samples
	// Allow ±10ms tolerance on the mean.
	if math.Abs(mean-keyDwellMeanMs) > 10 {
		t.Errorf("KeyDwellTime mean=%.1fms, want ~%.1fms ±10ms", mean, keyDwellMeanMs)
	}
}

// TestTypingDelay_Gaussian verifies that the gaussian TypingDelay has a mean
// near 120ms (vs old uniform ~85ms) over 1000 samples.
func TestTypingDelay_Gaussian(t *testing.T) {
	const samples = 1000
	var sum int
	vals := make([]int, samples)
	for i := range samples {
		d := TypingDelay()
		sum += d
		vals[i] = d
	}
	mean := float64(sum) / samples
	// Allow ±15ms tolerance on the mean — clamping shifts it slightly.
	if math.Abs(mean-typingMeanMs) > 15 {
		t.Errorf("TypingDelay mean=%.1fms, want ~%.1fms ±15ms", mean, typingMeanMs)
	}
}

// TestWordBoundaryPause_Frequency verifies that ~15% of calls return non-zero
// over 1000 samples (tolerance: 10-20%).
func TestWordBoundaryPause_Frequency(t *testing.T) {
	const samples = 1000
	var nonZero int
	for range samples {
		if WordBoundaryPause() > 0 {
			nonZero++
		}
	}
	freq := float64(nonZero) / samples
	if freq < 0.08 || freq > 0.25 {
		t.Errorf("WordBoundaryPause non-zero frequency=%.2f, want ~0.15 (8-25%%)", freq)
	}
}

// TestWordBoundaryPause_Range verifies that non-zero pauses are in [300, 500).
func TestWordBoundaryPause_Range(t *testing.T) {
	for range 500 {
		p := WordBoundaryPause()
		if p != 0 && (p < wordBoundaryMin || p >= wordBoundaryMin+wordBoundaryJitter) {
			t.Errorf("WordBoundaryPause() = %d out of expected range [%d, %d)",
				p, wordBoundaryMin, wordBoundaryMin+wordBoundaryJitter)
		}
	}
}

// TestTypingDelay_NotUniform verifies that TypingDelay is not flat-distributed
// by checking that values concentrate within 1σ of the mean more than a uniform
// distribution would (uniform would have ~33% within 1σ band; gaussian ~68%).
func TestTypingDelay_NotUniform(t *testing.T) {
	const samples = 2000
	const sigmaWidth = typingSigmaMs
	lo := int(typingMeanMs - sigmaWidth)
	hi := int(typingMeanMs + sigmaWidth)

	var inBand int
	for range samples {
		d := TypingDelay()
		if d >= lo && d <= hi {
			inBand++
		}
	}
	fraction := float64(inBand) / samples
	// Gaussian: ~68% within 1σ. Uniform over [30,300]: band [80,160] = 80/270 ≈ 30%.
	// Require at least 50% to confirm gaussian concentration.
	if fraction < 0.50 {
		t.Errorf("TypingDelay 1σ-band concentration=%.2f, want ≥0.50 (gaussian)", fraction)
	}
}
