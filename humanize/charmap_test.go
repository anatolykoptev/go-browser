package humanize

import "testing"

// TestLookupChar_AllPrintableASCII verifies every printable ASCII character
// (32-126) returns a non-zero VK code.
func TestLookupChar_AllPrintableASCII(t *testing.T) {
	for ch := rune(32); ch <= 126; ch++ {
		info := LookupChar(ch)
		if info.VK == 0 {
			t.Errorf("LookupChar(%q): VK=0, want non-zero", ch)
		}
	}
}

// TestLookupChar_ShiftedDigits verifies that !@#$%^&*() all have Shift=true.
func TestLookupChar_ShiftedDigits(t *testing.T) {
	shifted := "!@#$%^&*()"
	for _, ch := range shifted {
		info := LookupChar(ch)
		if !info.Shift {
			t.Errorf("LookupChar(%q): Shift=false, want true", ch)
		}
		if info.VK == 0 {
			t.Errorf("LookupChar(%q): VK=0, want non-zero", ch)
		}
	}
}

// TestLookupChar_OEMKeys verifies common OEM keys return correct VK codes.
func TestLookupChar_OEMKeys(t *testing.T) {
	tests := []struct {
		ch       rune
		wantVK   int
		wantCode string
	}{
		{';', 0xBA, "Semicolon"},
		{'=', 0xBB, "Equal"},
		{',', 0xBC, "Comma"},
		{'-', 0xBD, "Minus"},
		{'.', 0xBE, "Period"},
		{'/', 0xBF, "Slash"},
		{'`', 0xC0, "Backquote"},
		{'[', 0xDB, "BracketLeft"},
		{'\\', 0xDC, "Backslash"},
		{']', 0xDD, "BracketRight"},
		{'\'', 0xDE, "Quote"},
	}
	for _, tc := range tests {
		info := LookupChar(tc.ch)
		if info.VK != tc.wantVK {
			t.Errorf("LookupChar(%q): VK=%d, want %d", tc.ch, info.VK, tc.wantVK)
		}
		if info.Code != tc.wantCode {
			t.Errorf("LookupChar(%q): Code=%q, want %q", tc.ch, info.Code, tc.wantCode)
		}
		if info.Shift {
			t.Errorf("LookupChar(%q): Shift=true, want false (unshifted OEM)", tc.ch)
		}
	}
}

// TestLookupChar_Letters verifies lowercase/uppercase letter mapping.
func TestLookupChar_Letters(t *testing.T) {
	for ch := 'a'; ch <= 'z'; ch++ {
		info := LookupChar(ch)
		wantVK := int(ch - 32)
		if info.VK != wantVK {
			t.Errorf("LookupChar(%q): VK=%d, want %d", ch, info.VK, wantVK)
		}
		if info.Shift {
			t.Errorf("LookupChar(%q): Shift=true, want false", ch)
		}
	}
	for ch := 'A'; ch <= 'Z'; ch++ {
		info := LookupChar(ch)
		if info.VK != int(ch) {
			t.Errorf("LookupChar(%q): VK=%d, want %d", ch, info.VK, int(ch))
		}
		if !info.Shift {
			t.Errorf("LookupChar(%q): Shift=false, want true", ch)
		}
	}
}

// TestLookupChar_Space verifies space maps correctly.
func TestLookupChar_Space(t *testing.T) {
	info := LookupChar(' ')
	if info.VK != 32 {
		t.Errorf("LookupChar(' '): VK=%d, want 32", info.VK)
	}
	if info.Code != "Space" {
		t.Errorf("LookupChar(' '): Code=%q, want 'Space'", info.Code)
	}
}
