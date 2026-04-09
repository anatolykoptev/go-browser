package selftest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const rebrowserWaitTimeout = 30 * time.Second

// rebrowserReadyJS returns non-null when botDetectorResults is populated.
const rebrowserReadyJS = `
(function() {
  if (window.botDetectorResults && Object.keys(window.botDetectorResults).length > 0)
    return JSON.stringify(window.botDetectorResults);
  // Fall back to DOM-based detection result
  var el = document.querySelector('.results, #results, [data-results]');
  return el ? el.textContent.trim() : null;
})()
`

// rebrowserExtractJS extracts the full botDetectorResults object.
const rebrowserExtractJS = `
(function() {
  if (window.botDetectorResults) return JSON.stringify(window.botDetectorResults);
  // Parse visible result cards using for loop (safer with stealth-patched prototypes)
  var out = {};
  var els = document.querySelectorAll('[class*="test"], [data-test-id]');
  for (var i = 0; i < els.length; i++) {
    var el = els[i];
    var name = el.getAttribute('data-test-id') || el.className;
    var cls = el.getAttribute('class') || '';
    var passed = cls.indexOf('pass') >= 0 || cls.indexOf('success') >= 0;
    var failed = cls.indexOf('fail') >= 0 || cls.indexOf('error') >= 0;
    if (name && (passed || failed)) out[name] = passed;
  }
  return Object.keys(out).length > 0 ? JSON.stringify(out) : null;
})()
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
	var rawJSON string
	for time.Now().Before(deadline) {
		val, err := page.Eval(rebrowserReadyJS)
		if err == nil && val != nil {
			s := strings.TrimSpace(val.Value.String())
			if s != "" && s != "null" {
				rawJSON = s
				break
			}
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("rebrowser: context cancelled while waiting")
		case <-time.After(500 * time.Millisecond):
		}
	}

	if rawJSON == "" {
		// Try one final extract pass.
		val, err := page.Eval(rebrowserExtractJS)
		if err != nil {
			return result, fmt.Errorf("rebrowser: eval extract: %w", err)
		}
		if val == nil {
			return result, fmt.Errorf("rebrowser: selector not found")
		}
		rawJSON = val.Value.String()
	}

	if rawJSON == "" || rawJSON == "null" {
		return result, fmt.Errorf("rebrowser: no results extracted — page structure may have changed")
	}

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
