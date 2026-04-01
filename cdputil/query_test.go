package cdputil

import (
	"testing"
)

func TestParseSelector(t *testing.T) {
	tests := []struct {
		input    string
		kind     selectorKind
		selector string
	}{
		{"#username", kindCSS, "#username"},
		{"text=Sign in", kindText, "Sign in"},
		{"xpath=//button", kindXPath, "//button"},
		{".login-form input[type=email]", kindCSS, ".login-form input[type=email]"},
	}
	for _, tt := range tests {
		kind, sel := parseSelector(tt.input)
		if kind != tt.kind {
			t.Errorf("parseSelector(%q) kind = %v, want %v", tt.input, kind, tt.kind)
		}
		if sel != tt.selector {
			t.Errorf("parseSelector(%q) selector = %q, want %q", tt.input, sel, tt.selector)
		}
	}
}
