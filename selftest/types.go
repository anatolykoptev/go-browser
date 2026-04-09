package selftest

import "time"

// Report is the top-level response for GET /selftest.
type Report struct {
	Profile   string         `json:"profile"`
	StartedAt time.Time      `json:"started_at"`
	Results   []TargetResult `json:"results"`
	Summary   Summary        `json:"summary"`
}

// TargetResult holds the outcome of running one probe target.
type TargetResult struct {
	Target         string         `json:"target"`
	URL            string         `json:"url"`
	DurationMs     int64          `json:"duration_ms"`
	OK             bool           `json:"ok"`
	TrustScore     float64        `json:"trust_score,omitempty"`
	Lies           []string       `json:"lies,omitempty"`
	Sections       map[string]any `json:"sections,omitempty"`
	ScreenshotPath string         `json:"screenshot_path,omitempty"`
	Error          string         `json:"error,omitempty"`
}

// Summary aggregates results across all targets.
type Summary struct {
	Total        int     `json:"total"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	OverallTrust float64 `json:"overall_trust"`
}
