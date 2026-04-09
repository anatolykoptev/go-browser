package selftest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildSummary verifies aggregate stats across mixed results.
func TestBuildSummary(t *testing.T) {
	t.Run("all passed with trust", func(t *testing.T) {
		results := []TargetResult{
			{OK: true, TrustScore: 90},
			{OK: true, TrustScore: 80},
		}
		s := buildSummary(results)
		if s.Total != 2 {
			t.Errorf("Total: want 2, got %d", s.Total)
		}
		if s.Passed != 2 {
			t.Errorf("Passed: want 2, got %d", s.Passed)
		}
		if s.Failed != 0 {
			t.Errorf("Failed: want 0, got %d", s.Failed)
		}
		if s.OverallTrust != 85.0 {
			t.Errorf("OverallTrust: want 85, got %f", s.OverallTrust)
		}
	})

	t.Run("mixed pass fail", func(t *testing.T) {
		results := []TargetResult{
			{OK: true, TrustScore: 100},
			{OK: false},
		}
		s := buildSummary(results)
		if s.Passed != 1 || s.Failed != 1 {
			t.Errorf("want 1 passed 1 failed, got %d/%d", s.Passed, s.Failed)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		s := buildSummary(nil)
		if s.Total != 0 || s.OverallTrust != 0 {
			t.Errorf("want zero summary, got %+v", s)
		}
	})

	t.Run("passed but zero trust score excluded from average", func(t *testing.T) {
		results := []TargetResult{
			{OK: true, TrustScore: 0},  // sannysoft-style: no score
			{OK: true, TrustScore: 90}, // creepjs-style: has score
		}
		s := buildSummary(results)
		if s.OverallTrust != 90 {
			t.Errorf("OverallTrust: want 90 (zero excluded), got %f", s.OverallTrust)
		}
	})
}

// TestResolveTargets verifies target resolution from keys.
func TestResolveTargets(t *testing.T) {
	t.Run("empty returns all", func(t *testing.T) {
		got := resolveTargets(nil)
		if len(got) != len(AllTargets) {
			t.Errorf("want %d targets, got %d", len(AllTargets), len(got))
		}
	})

	t.Run("specific keys", func(t *testing.T) {
		got := resolveTargets([]string{"creepjs", "canvas"})
		if len(got) != 2 {
			t.Errorf("want 2 targets, got %d", len(got))
		}
		if got[0].Key != "creepjs" || got[1].Key != "canvas" {
			t.Errorf("unexpected keys: %v", got)
		}
	})

	t.Run("unknown key is skipped", func(t *testing.T) {
		got := resolveTargets([]string{"creepjs", "nonexistent"})
		if len(got) != 1 || got[0].Key != "creepjs" {
			t.Errorf("want only creepjs, got %v", got)
		}
	})
}

// TestAllTargetsRegistry verifies the target and extractor registries are consistent.
func TestAllTargetsRegistry(t *testing.T) {
	for _, tgt := range AllTargets {
		if tgt.Key == "" {
			t.Errorf("target has empty key: %+v", tgt)
		}
		if tgt.URL == "" {
			t.Errorf("target %q has empty URL", tgt.Key)
		}
		if _, ok := Extractors[tgt.Key]; !ok {
			t.Errorf("target %q has no registered extractor", tgt.Key)
		}
	}
	// Verify count matches.
	if len(Extractors) != len(AllTargets) {
		t.Errorf("Extractors count %d != AllTargets count %d", len(Extractors), len(AllTargets))
	}
}

// TestCreepJSResultParsing tests that the creepjs JSON fixture parses into the expected shape.
func TestCreepJSResultParsing(t *testing.T) {
	data := loadFixture(t, "creepjs_result.json")
	var cr creepJSResult
	if err := json.Unmarshal(data, &cr); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if cr.TrustScore != 94.5 {
		t.Errorf("TrustScore: want 94.5, got %f", cr.TrustScore)
	}
	if len(cr.Lies) != 0 {
		t.Errorf("Lies: want 0, got %d", len(cr.Lies))
	}
	if cr.Fonts.Hash == "" {
		t.Error("Fonts.Hash should not be empty")
	}
	if cr.Fonts.PlatformClassifier != "Apple" {
		t.Errorf("PlatformClassifier: want Apple, got %s", cr.Fonts.PlatformClassifier)
	}
	if cr.Audio.Hash == "" {
		t.Error("Audio.Hash should not be empty")
	}
	if cr.Voices.Count != 34 {
		t.Errorf("Voices.Count: want 34, got %d", cr.Voices.Count)
	}
	if cr.UA.Platform != "macOS" {
		t.Errorf("UA.Platform: want macOS, got %s", cr.UA.Platform)
	}
}

// TestSannysoftChecksParsing tests that the sannysoft JSON fixture parses correctly.
func TestSannysoftChecksParsing(t *testing.T) {
	data := loadFixture(t, "sannysoft_checks.json")
	var checks sannysoftChecks
	if err := json.Unmarshal(data, &checks); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(checks.Checks) != 6 {
		t.Errorf("checks count: want 6, got %d", len(checks.Checks))
	}
	if checks.Passed != 5 {
		t.Errorf("Passed: want 5, got %d", checks.Passed)
	}
	if checks.Failed != 1 {
		t.Errorf("Failed: want 1, got %d", checks.Failed)
	}

	// Verify trust score computation: 5/6 * 100 ≈ 83.33
	total := checks.Passed + checks.Failed
	score := float64(checks.Passed) / float64(total) * maxTrustScore
	if score < 83.0 || score > 84.0 {
		t.Errorf("score: want ~83.33, got %f", score)
	}
}

// TestReportJSONShape verifies the Report struct serialises to expected JSON keys.
func TestReportJSONShape(t *testing.T) {
	r := Report{
		Profile: "mac_chrome145",
		Results: []TargetResult{
			{Target: "creepjs", URL: "https://example.com", OK: true, TrustScore: 90},
		},
		Summary: Summary{Total: 1, Passed: 1, OverallTrust: 90},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"profile", "started_at", "results", "summary"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key %q in Report", key)
		}
	}
}

// loadFixture reads a file from the testdata directory.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return data
}
