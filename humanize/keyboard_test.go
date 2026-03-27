package humanize

import "testing"

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
