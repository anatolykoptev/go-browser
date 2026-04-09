package selftest

import (
	"encoding/json"
	"testing"
)

// TestRebrowserResultsParsing verifies the rebrowser JSON fixture parses correctly.
func TestRebrowserResultsParsing(t *testing.T) {
	data := loadFixture(t, "rebrowser_results.json")
	var checks map[string]any
	if err := json.Unmarshal(data, &checks); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(checks) == 0 {
		t.Error("expected at least one check")
	}

	// All values false => no bot detected.
	botDetected := false
	for _, v := range checks {
		if b, ok := v.(bool); ok && b {
			botDetected = true
		}
	}
	if botDetected {
		t.Error("fixture should have no bot signals (all false)")
	}
}

// TestRebrowserBotDetectedLogic verifies that a true value triggers bot detection.
func TestRebrowserBotDetectedLogic(t *testing.T) {
	checks := map[string]any{
		"webdriverPresent": true,
		"headlessChrome":   false,
	}

	botDetected := false
	for _, v := range checks {
		if b, ok := v.(bool); ok && b {
			botDetected = true
			break
		}
	}
	if !botDetected {
		t.Error("expected botDetected=true when webdriverPresent=true")
	}
}

// TestBotDVerdictParsing verifies the botd JSON fixture parses correctly.
func TestBotDVerdictParsing(t *testing.T) {
	data := loadFixture(t, "botd_verdict.json")
	var verdict struct {
		Bot        *bool  `json:"bot"`
		Confidence *int   `json:"confidence"`
		Raw        string `json:"raw"`
	}
	if err := json.Unmarshal(data, &verdict); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if verdict.Bot == nil {
		t.Fatal("bot field should not be nil")
	}
	if *verdict.Bot {
		t.Error("fixture should classify as not-bot")
	}
	if verdict.Raw == "" {
		t.Error("raw verdict text should not be empty")
	}
}

// TestWebRTCLeakParsing verifies the webrtc_ips fixture parses correctly.
func TestWebRTCLeakParsing(t *testing.T) {
	data := loadFixture(t, "webrtc_ips.json")
	var ips struct {
		PublicIPs []string `json:"publicIps"`
		LocalIPs  []string `json:"localIps"`
		AllIPs    []string `json:"allIps"`
	}
	if err := json.Unmarshal(data, &ips); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(ips.LocalIPs) != 0 {
		t.Errorf("expected no local IPs, got %v", ips.LocalIPs)
	}
	if len(ips.PublicIPs) != 1 {
		t.Errorf("expected 1 public IP, got %d", len(ips.PublicIPs))
	}
}

// TestWebRTCLeakDetection verifies RFC1918 detection logic.
func TestWebRTCLeakDetection(t *testing.T) {
	cases := []struct {
		ip      string
		isLocal bool
	}{
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.15.0.1", false}, // just outside range
		{"185.100.200.50", false},
		{"8.8.8.8", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			isLocal := isRFC1918(tc.ip)
			if isLocal != tc.isLocal {
				t.Errorf("isRFC1918(%s): want %v, got %v", tc.ip, tc.isLocal, isLocal)
			}
		})
	}
}

// TestCanvasHashParsing verifies the canvas fixture parses correctly.
func TestCanvasHashParsing(t *testing.T) {
	data := loadFixture(t, "canvas_hash.json")
	var canvasData struct {
		Hash       string `json:"hash"`
		Uniqueness string `json:"uniqueness"`
		Raw        string `json:"raw"`
	}
	if err := json.Unmarshal(data, &canvasData); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if canvasData.Hash == "" {
		t.Error("canvas hash should not be empty")
	}
	if canvasData.Uniqueness == "" {
		t.Error("uniqueness should not be empty")
	}
}
