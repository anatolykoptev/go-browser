package browser

import "testing"

func TestResolveSessionParams_EmptySessionRespectsMode(t *testing.T) {
	cases := []struct {
		name          string
		req           InteractRequest
		wantMode      string
		wantEphemeral bool
	}{
		{"empty session + mode=default", InteractRequest{Mode: "default"}, "default", true},
		{"empty session + mode=private", InteractRequest{Mode: "private"}, "private", true},
		{"empty session + mode=proxy", InteractRequest{Mode: "proxy", Proxy: strPtr("http://p:80")}, "proxy", true},
		{"explicit session + mode=default", InteractRequest{Session: "foo", Mode: "default"}, "default", false},
		{"no session no mode", InteractRequest{}, "private", false}, // backward compat
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, mode, _, eph := resolveSessionParams(tc.req)
			if mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", mode, tc.wantMode)
			}
			if eph != tc.wantEphemeral {
				t.Errorf("ephemeral = %v, want %v", eph, tc.wantEphemeral)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
