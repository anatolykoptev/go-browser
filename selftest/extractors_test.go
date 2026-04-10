package selftest

import (
	"encoding/json"
	"strings"
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

// TestIncolumitasNewTestsParsing verifies new_tests fixture parses all checks as "OK".
func TestIncolumitasNewTestsParsing(t *testing.T) {
	data := loadFixture(t, "incolumitas_new_tests.json")
	var checks map[string]string
	if err := json.Unmarshal(data, &checks); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(checks) == 0 {
		t.Fatal("expected at least one check")
	}
	for k, v := range checks {
		if v != "OK" {
			t.Errorf("check %q: want OK, got %q", k, v)
		}
	}
}

// TestIncolumitasCheckCounting verifies OK/FAIL counting via parseIncolumitasChecks.
func TestIncolumitasCheckCounting(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantOK    int
		wantFail  int
		wantNilOK bool
	}{
		{
			name:     "all OK",
			raw:      `{"puppeteerEvaluationScript":"OK","webdriverPresent":"OK","connectionRTT":"OK"}`,
			wantOK:   3,
			wantFail: 0,
		},
		{
			name:     "mixed",
			raw:      `{"puppeteerEvaluationScript":"OK","webdriverPresent":"FAIL","connectionRTT":"OK"}`,
			wantOK:   2,
			wantFail: 1,
		},
		{
			name:      "nil input",
			raw:       "",
			wantNilOK: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rawPtr *string
			if tc.raw != "" {
				rawPtr = &tc.raw
			}
			m, ok, fail := parseIncolumitasChecks(rawPtr)
			if tc.wantNilOK {
				if m != nil || ok != 0 || fail != 0 {
					t.Errorf("want nil result, got m=%v ok=%d fail=%d", m, ok, fail)
				}
				return
			}
			if ok != tc.wantOK {
				t.Errorf("okCount: want %d, got %d", tc.wantOK, ok)
			}
			if fail != tc.wantFail {
				t.Errorf("failCount: want %d, got %d", tc.wantFail, fail)
			}
		})
	}
}

// TestIncolumitasFlatCheckCounting verifies flat check counting via parseIncolumitasFlatChecks.
func TestIncolumitasFlatCheckCounting(t *testing.T) {
	data := loadFixture(t, "incolumitas_fpscanner.json")
	raw := string(data)
	m, ok, fail := parseIncolumitasFlatChecks(&raw)
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if ok == 0 {
		t.Error("expected at least one OK check")
	}
	if fail != 0 {
		t.Errorf("expected zero FAIL checks, got %d", fail)
	}
	total := ok + fail
	score := float64(ok) / float64(total) * maxTrustScore
	if score != maxTrustScore {
		t.Errorf("score: want 100.0, got %f", score)
	}
}

// TestIncolumitasIPInfoParsing verifies IP info fixture parses correctly.
func TestIncolumitasIPInfoParsing(t *testing.T) {
	data := loadFixture(t, "incolumitas_ip_info.json")
	var ipInfo map[string]any
	if err := json.Unmarshal(data, &ipInfo); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if _, ok := ipInfo["is_datacenter"]; !ok {
		t.Error("expected is_datacenter field")
	}
	if _, ok := ipInfo["asn"]; !ok {
		t.Error("expected asn field")
	}
}

// TestBrowserScanResultParsing verifies the browserscan fixture parses correctly.
func TestBrowserScanResultParsing(t *testing.T) {
	data := loadFixture(t, "browserscan_result.json")
	var result browserscanData
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if result.Verdict == "" {
		t.Error("verdict should not be empty")
	}
	if len(result.Checks) == 0 {
		t.Error("expected at least one check")
	}
	if result.Normal == 0 {
		t.Error("expected at least one Normal check")
	}
}

// TestBrowserScanTrustScore verifies trust score computation from Normal/Abnormal counts.
func TestBrowserScanTrustScore(t *testing.T) {
	cases := []struct {
		name      string
		normal    int
		abnormal  int
		wantScore float64
	}{
		{"all normal", 8, 0, 100.0},
		{"all abnormal", 0, 8, 0.0},
		{"7 of 8", 7, 1, 87.5},
		{"zero total", 0, 0, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			total := tc.normal + tc.abnormal
			var score float64
			if total > 0 {
				score = float64(tc.normal) / float64(total) * maxTrustScore
			}
			if score != tc.wantScore {
				t.Errorf("score: want %f, got %f", tc.wantScore, score)
			}
		})
	}
}

// TestBrowserScanVerdictBotFlag verifies bot verdict triggers Lies field.
func TestBrowserScanVerdictBotFlag(t *testing.T) {
	cases := []struct {
		verdict  string
		wantLies bool
	}{
		{"No bots detected", false},
		{"Robot", true},
		{"Bot detected", true},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.verdict, func(t *testing.T) {
			lower := strings.ToLower(tc.verdict)
			isBot := strings.Contains(lower, "robot") || strings.Contains(lower, "bot detected")
			if isBot != tc.wantLies {
				t.Errorf("verdict %q: wantLies=%v, got %v", tc.verdict, tc.wantLies, isBot)
			}
		})
	}
}
