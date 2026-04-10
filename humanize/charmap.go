package humanize

import "strings"

// CharInfo holds the Windows Virtual Key code, DOM KeyboardEvent.code,
// and Shift modifier flag for a single character.
type CharInfo struct {
	VK    int    // Windows Virtual Key code
	Code  string // DOM KeyboardEvent.code
	Shift bool   // requires Shift modifier
}

// LookupChar returns the CharInfo for the given rune.
// Unmapped characters return VK=int(ch), Code="", Shift=false.
func LookupChar(ch rune) CharInfo {
	if info, ok := charTable[ch]; ok {
		return info
	}
	return CharInfo{VK: int(ch)}
}

// charTable is the complete printable-ASCII mapping derived from
// Chromium keyboard_codes_posix.h and the DOM Level 3 KeyboardEvent spec.
var charTable = buildCharTable()

func buildCharTable() map[rune]CharInfo {
	m := make(map[rune]CharInfo, 96) //nolint:mnd // 96 printable ASCII chars

	// Letters a-z
	for ch := 'a'; ch <= 'z'; ch++ {
		code := "Key" + strings.ToUpper(string(ch))
		m[ch] = CharInfo{VK: int(ch - 32), Code: code}
	}
	// Letters A-Z (Shift)
	for ch := 'A'; ch <= 'Z'; ch++ {
		code := "Key" + string(ch)
		m[ch] = CharInfo{VK: int(ch), Code: code, Shift: true}
	}

	// Digits 0-9
	for ch := '0'; ch <= '9'; ch++ {
		code := "Digit" + string(ch)
		m[ch] = CharInfo{VK: int(ch), Code: code}
	}

	// Shifted digits: !@#$%^&*()
	shiftedDigits := []struct {
		ch   rune
		vk   int
		code string
	}{
		{'!', 49, "Digit1"},
		{'@', 50, "Digit2"},
		{'#', 51, "Digit3"},
		{'$', 52, "Digit4"},
		{'%', 53, "Digit5"},
		{'^', 54, "Digit6"},
		{'&', 55, "Digit7"},
		{'*', 56, "Digit8"},
		{'(', 57, "Digit9"},
		{')', 48, "Digit0"},
	}
	for _, sd := range shiftedDigits {
		m[sd.ch] = CharInfo{VK: sd.vk, Code: sd.code, Shift: true}
	}

	// OEM keys (unshifted / shifted pairs share the same VK)
	oemKeys := []struct {
		unshifted rune
		shifted   rune
		vk        int
		code      string
	}{
		{';', ':', 0xBA, "Semicolon"},
		{'=', '+', 0xBB, "Equal"},
		{',', '<', 0xBC, "Comma"},
		{'-', '_', 0xBD, "Minus"},
		{'.', '>', 0xBE, "Period"},
		{'/', '?', 0xBF, "Slash"},
		{'`', '~', 0xC0, "Backquote"},
		{'[', '{', 0xDB, "BracketLeft"},
		{'\\', '|', 0xDC, "Backslash"},
		{']', '}', 0xDD, "BracketRight"},
		{'\'', '"', 0xDE, "Quote"},
	}
	for _, ok := range oemKeys {
		m[ok.unshifted] = CharInfo{VK: ok.vk, Code: ok.code}
		m[ok.shifted] = CharInfo{VK: ok.vk, Code: ok.code, Shift: true}
	}

	// Control / whitespace keys
	m[' '] = CharInfo{VK: 32, Code: "Space"}
	m['\t'] = CharInfo{VK: 9, Code: "Tab"}
	m['\r'] = CharInfo{VK: 13, Code: "Enter"}
	m['\n'] = CharInfo{VK: 13, Code: "Enter"}
	m['\b'] = CharInfo{VK: 8, Code: "Backspace"}

	return m
}
