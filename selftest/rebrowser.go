package selftest

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

const rebrowserWaitTimeout = 30 * time.Second

// rebrowserReadyJS polls for window.botDetectorResults or DOM results.
// Returns a sentinel "ready" string when results are available.
const rebrowserReadyJS = `
() => {
  try {
    var r = window.botDetectorResults;
    if (r && typeof r === 'object' && Object.keys(r).length > 0) return 'ready';
  } catch(e) {}
  // DOM fallback: rebrowser renders a results table when JS finishes
  var rows = document.querySelectorAll('table tr, .result-row, [class*="result"]');
  if (rows.length > 2) return 'ready-dom';
  // Check for any text indicating test completed
  var body = document.body ? document.body.innerText : '';
  if (body.length > 500 && (body.indexOf('headless') >= 0 || body.indexOf('automation') >= 0 || body.indexOf('bot') >= 0)) return 'ready-text';
  return null;
}
`

// rebrowserExtractJS extracts the full botDetectorResults object as JSON.
const rebrowserExtractJS = `
() => {
  var out = { checks: {}, _debug: '', _source: 'none' };

  // Try window.botDetectorResults first
  try {
    var r = window.botDetectorResults;
    if (r && typeof r === 'object' && Object.keys(r).length > 0) {
      out.checks = r;
      out._source = 'window';
      out._debug = 'found window.botDetectorResults with ' + Object.keys(r).length + ' keys';
      return JSON.stringify(out);
    }
  } catch(e) { out._debug = 'window error: ' + e.message; }

  // DOM fallback: parse result table rows
  var rows = document.querySelectorAll('table tr');
  for (var i = 0; i < rows.length; i++) {
    var cells = rows[i].querySelectorAll('td, th');
    if (cells.length >= 2) {
      var k = cells[0].textContent.trim();
      var v = cells[1].textContent.trim();
      if (k) out.checks[k] = (v === 'true' || v === 'detected' || v === 'yes');
    }
  }

  // Capture page text for debugging
  out._debug = (out._debug || '') + ' | DOM rows: ' + rows.length;
  out._source = rows.length > 0 ? 'dom' : 'none';
  if (document.body) out._debug += ' | body[0:300]: ' + document.body.innerText.substring(0, 300);

  if (Object.keys(out.checks).length === 0) return null;
  return JSON.stringify(out);
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

	// Response is either {checks:{}, _debug:'', _source:''} (new) or map[string]bool (old).
	var envelope struct {
		Checks map[string]any `json:"checks"`
		Debug  string         `json:"_debug"`
		Source string         `json:"_source"`
	}
	var checks map[string]any
	if err := parseJSON(rawJSON, &envelope); err != nil {
		return result, fmt.Errorf("rebrowser: parse result: %w", err)
	}
	if envelope.Checks != nil {
		checks = envelope.Checks
	} else {
		// Legacy format: top-level map[string]bool
		if err := parseJSON(rawJSON, &checks); err != nil {
			return result, fmt.Errorf("rebrowser: parse legacy result: %w", err)
		}
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
	sections := map[string]any{
		"checks":      checks,
		"botDetected": botDetected,
	}
	if envelope.Debug != "" {
		sections["_debug"] = envelope.Debug
	}
	if envelope.Source != "" {
		sections["_source"] = envelope.Source
	}
	result.Sections = sections
	return result, nil
}
