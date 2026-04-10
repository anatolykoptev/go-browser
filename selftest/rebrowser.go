package selftest

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

const rebrowserWaitTimeout = 30 * time.Second

// rebrowserReadyJS polls for window.botDetectorResults to be populated.
// Returns a sentinel "ready" string when results are available.
const rebrowserReadyJS = `
() => {
  try {
    var r = window.botDetectorResults;
    if (r && typeof r === 'object' && Object.keys(r).length > 0) return 'ready';
  } catch(e) {}
  return null;
}
`

// rebrowserExtractJS extracts the full botDetectorResults object as JSON.
const rebrowserExtractJS = `
() => {
  try {
    var r = window.botDetectorResults;
    if (r && typeof r === 'object') return JSON.stringify(r);
  } catch(e) {}
  return null;
}
`

// extractRebrowser reads window.botDetectorResults from https://bot-detector.rebrowser.net/
//
// Strategy: wait up to 30 s for window.botDetectorResults to appear, then extract.
// A result is considered passing when all detected checks are false (not-bot).
func extractRebrowser(ctx context.Context, page *rod.Page) (TargetResult, error) {
	result := TargetResult{
		Target: "rebrowser",
		URL:    "https://bot-detector.rebrowser.net/",
	}

	deadline := time.Now().Add(rebrowserWaitTimeout)
	// Wait for botDetectorResults to be populated.
	for time.Now().Before(deadline) {
		val, err := page.Eval(rebrowserReadyJS)
		if err == nil && val != nil && !isNullResult(val.Value.String()) {
			break
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("rebrowser: context cancelled while waiting")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Extract the JSON now that results are ready.
	val, err := page.Eval(rebrowserExtractJS)
	if err != nil {
		return result, fmt.Errorf("rebrowser: eval extract: %w", err)
	}
	if val == nil || isNullResult(val.Value.String()) {
		return result, fmt.Errorf("rebrowser: botDetectorResults not found — page structure may have changed")
	}
	rawJSON := val.Value.String()

	// botDetectorResults is a map[string]bool: key=check name, value=true means bot detected.
	var checks map[string]any
	if err := parseJSON(rawJSON, &checks); err != nil {
		return result, fmt.Errorf("rebrowser: parse result: %w", err)
	}

	botDetected := false
	for _, v := range checks {
		if b, ok := v.(bool); ok && b {
			botDetected = true
			break
		}
	}

	// Trust: 100 if no bot signals detected, 0 otherwise.
	var trustScore float64
	if !botDetected {
		trustScore = maxTrustScore
	}

	result.OK = !botDetected
	result.TrustScore = trustScore
	result.Sections = map[string]any{
		"checks":      checks,
		"botDetected": botDetected,
	}
	return result, nil
}
