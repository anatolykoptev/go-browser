package humanize

import (
	"testing"
)

// TestQWERTYAdjacent verifies adjacency entries for 'e', 'a', and ' ' (space).
func TestQWERTYAdjacent(t *testing.T) {
	cases := []struct {
		key      rune
		mustHave []rune
	}{
		{'e', []rune{'w', 'r', 'd'}},
		{'a', []rune{'q', 's', 'z'}},
		{' ', []rune{'b', 'n', 'v'}},
	}

	for _, tc := range cases {
		neighbors := qwertyAdjacent[tc.key]
		neighborSet := make(map[rune]bool, len(neighbors))
		for _, n := range neighbors {
			neighborSet[n] = true
		}
		for _, want := range tc.mustHave {
			if !neighborSet[want] {
				t.Errorf("qwertyAdjacent[%q] missing %q; got %q", tc.key, want, neighbors)
			}
		}
	}
}

// TestShouldTypo_Rate verifies the 3% typo rate over 10000 samples (tolerance ±1.5%).
func TestShouldTypo_Rate(t *testing.T) {
	const (
		samples  = 10_000
		wantLow  = 0.015
		wantHigh = 0.045
	)

	var count int
	for range samples {
		if ShouldTypo() {
			count++
		}
	}

	rate := float64(count) / samples
	if rate < wantLow || rate > wantHigh {
		t.Errorf("ShouldTypo rate=%.4f, want [%.3f, %.3f]", rate, wantLow, wantHigh)
	}
}

// TestTypoChar_WeightedDistribution verifies that adjacent_key is the most
// frequent variant over 1000 samples for a mid-keyboard key.
func TestTypoChar_WeightedDistribution(t *testing.T) {
	const samples = 1000
	correct := 'f' // has rich adjacency: d,r,t,g,c,v

	neighbors := make(map[rune]bool)
	for _, n := range qwertyAdjacent[correct] {
		neighbors[n] = true
	}

	adjacentCount := 0
	for range samples {
		got := TypoChar(correct)
		if neighbors[got] {
			adjacentCount++
		}
	}

	// Adjacent_key weight is 55% — expect at least 40% (robust lower bound).
	rate := float64(adjacentCount) / samples
	if rate < 0.40 {
		t.Errorf("TypoChar adjacent_key rate=%.2f, want ≥0.40 (weight=55%%)", rate)
	}
}

// TestTypoChar_SkipReturnsZero verifies that skip variant returns 0.
// We test indirectly: TypoChar must be capable of returning 0 for 'f'.
// Drive enough samples to observe at least one skip (8% weight).
func TestTypoChar_SkipReturnsZero(t *testing.T) {
	const samples = 2000
	var gotZero bool
	for range samples {
		if TypoChar('f') == 0 {
			gotZero = true
			break
		}
	}
	if !gotZero {
		t.Error("TypoChar never returned 0 (skip variant) in 2000 samples")
	}
}

// TestTypoCorrection_BackspacePresent verifies that correction contains exactly
// one Backspace event.
func TestTypoCorrection_BackspacePresent(t *testing.T) {
	for trial := range 50 {
		events := TypoCorrection('a')

		var backspaceCount int
		for _, ev := range events {
			if ev.IsBackspace {
				backspaceCount++
				if ev.Char != 0 {
					t.Errorf("trial %d: Backspace event has non-zero Char=%q", trial, ev.Char)
				}
			}
		}

		if backspaceCount != 1 {
			t.Errorf("trial %d: got %d Backspace events, want exactly 1", trial, backspaceCount)
		}
	}
}

// TestTypoCorrection_DelayRanges verifies all delay/dwell values are in their spec ranges.
func TestTypoCorrection_DelayRanges(t *testing.T) {
	for range 200 {
		events := TypoCorrection('e')

		if len(events) != 2 {
			t.Fatalf("TypoCorrection: got %d events, want 2", len(events))
		}

		bs := events[0]
		if !bs.IsBackspace {
			t.Error("events[0] should be Backspace")
		}
		if bs.DelayMs < typoRealizePauseMin || bs.DelayMs > typoRealizePauseMin+typoRealizePauseMax {
			t.Errorf("Backspace DelayMs=%d out of [%d, %d]",
				bs.DelayMs, typoRealizePauseMin, typoRealizePauseMin+typoRealizePauseMax)
		}
		if bs.DwellMs < typoBackspaceDwellMin || bs.DwellMs > typoBackspaceDwellMin+typoBackspaceDwellMax {
			t.Errorf("Backspace DwellMs=%d out of [%d, %d]",
				bs.DwellMs, typoBackspaceDwellMin, typoBackspaceDwellMin+typoBackspaceDwellMax)
		}

		ch := events[1]
		if ch.IsBackspace {
			t.Error("events[1] should not be Backspace")
		}
		if ch.Char != 'e' {
			t.Errorf("events[1].Char=%q, want 'e'", ch.Char)
		}
		if ch.DelayMs < typoPostBackspacePauseMin || ch.DelayMs > typoPostBackspacePauseMin+typoPostBackspacePauseMax {
			t.Errorf("correct char DelayMs=%d out of [%d, %d]",
				ch.DelayMs, typoPostBackspacePauseMin, typoPostBackspacePauseMin+typoPostBackspacePauseMax)
		}
	}
}
