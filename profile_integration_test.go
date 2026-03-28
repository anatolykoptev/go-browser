package browser

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileInjectJS(t *testing.T) {
	for _, name := range ListProfiles() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadProfile(name)
			if err != nil {
				t.Fatalf("load %q: %v", name, err)
			}

			js := p.InjectJS()
			if js == "" {
				t.Fatal("empty InjectJS")
			}

			prefix := "window.__stealthProfile = "
			suffix := "; window.__sp = window.__stealthProfile;"
			if !strings.HasPrefix(js, prefix) || !strings.HasSuffix(js, suffix) {
				t.Fatalf("unexpected InjectJS format: %s", js[:50])
			}
			raw := js[len(prefix) : len(js)-len(suffix)]

			var check StealthProfile
			if err := json.Unmarshal([]byte(raw), &check); err != nil {
				t.Fatalf("InjectJS JSON invalid: %v", err)
			}

			if check.OS == "" {
				t.Error("OS empty")
			}
			if check.GPU.Renderer == "" {
				t.Error("GPU renderer empty")
			}
			if check.Platform == "" {
				t.Error("Platform empty")
			}
			if len(check.UAData.Brands) == 0 {
				t.Error("no UA brands")
			}
		})
	}
}

func TestListProfiles(t *testing.T) {
	profiles := ListProfiles()
	if len(profiles) < 3 {
		t.Errorf("expected at least 3 profiles, got %d", len(profiles))
	}
}

func TestInteractRequestProfile(t *testing.T) {
	raw := `{"url":"https://example.com","profile":"win_chrome145","actions":[]}`
	var req InteractRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	if req.Profile != "win_chrome145" {
		t.Errorf("Profile = %q, want win_chrome145", req.Profile)
	}
}
