package browser

import (
	"strings"
	"testing"
)

func TestLoadProfileDefault(t *testing.T) {
	p, err := LoadProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if p.OS != "macos" {
		t.Errorf("default profile OS = %q, want macos", p.OS)
	}
	if p.GPU.Renderer == "" {
		t.Error("GPU renderer empty")
	}
}

func TestLoadProfileByName(t *testing.T) {
	p, err := LoadProfile("win_chrome145")
	if err != nil {
		t.Fatal(err)
	}
	if p.OS != "windows" {
		t.Errorf("OS = %q, want windows", p.OS)
	}
	if p.Platform != "Win32" {
		t.Errorf("Platform = %q, want Win32", p.Platform)
	}
	if p.GPU.Vendor == "" {
		t.Error("GPU vendor empty")
	}
}

func TestLoadProfileInvalid(t *testing.T) {
	_, err := LoadProfile("nonexistent_profile")
	if err == nil {
		t.Error("expected error for unknown profile")
	}
}

func TestProfileToJS(t *testing.T) {
	p, _ := LoadProfile("mac_chrome145")
	js := p.InjectJS()
	if js == "" {
		t.Error("InjectJS returned empty")
	}
	if !strings.Contains(js, "MacIntel") {
		t.Error("InjectJS missing platform")
	}
	if !strings.Contains(js, "Intel Iris") {
		t.Error("InjectJS missing GPU renderer")
	}
}
