package browser

import (
	"strings"
	"testing"
)

func TestChromeManager_VersionURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ws with port",
			input: "ws://127.0.0.1:9222",
			want:  "http://127.0.0.1:9222/json/version",
		},
		{
			name:  "ws with host",
			input: "ws://cloakbrowser:9222",
			want:  "http://cloakbrowser:9222/json/version",
		},
		{
			name:  "wss scheme",
			input: "wss://example.com:9222",
			want:  "https://example.com:9222/json/version",
		},
		{
			name:  "trailing slash stripped",
			input: "ws://127.0.0.1:9222/",
			want:  "http://127.0.0.1:9222/json/version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionURL(tt.input)
			if got != tt.want {
				t.Errorf("versionURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestChromeManager_StealthJS(t *testing.T) {
	if complementJS == "" {
		t.Fatal("complementJS is empty — embed directive not working")
	}

	patterns := []string{
		"webdriver",
		"chrome.runtime",
		"canPlayType",
		"OriginalWorker",
		"__cdp_runtime",
	}

	for _, p := range patterns {
		t.Run("contains_"+p, func(t *testing.T) {
			if !strings.Contains(complementJS, p) {
				t.Errorf("stealth_complement.js missing expected pattern %q", p)
			}
		})
	}
}
