package selftest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const browserscanWaitTimeout = 20 * time.Second

// browserscanReadyJS returns non-null once "Normal" appears in the page text,
// indicating the check table has been rendered.
const browserscanReadyJS = `
() => {
  var text = document.body ? document.body.innerText : '';
  return text.indexOf('Normal') >= 0 ? text : null;
}
`

// browserscanExtractJS extracts check names + statuses and the top verdict.
const browserscanExtractJS = `
() => {
  var text = document.body ? document.body.innerText : '';
  if (!text) return null;

  var lines = text.split('\n').map(function(l) { return l.trim(); }).filter(function(l) { return l.length > 0; });
  var checks = {};
  var normalCount = 0;
  var abnormalCount = 0;

  // Each check: the line before "Normal" or "Abnormal" is the check name.
  for (var i = 1; i < lines.length; i++) {
    var status = lines[i];
    if (status === 'Normal' || status === 'Abnormal') {
      var name = lines[i - 1];
      // Skip lines that are themselves status words or UI chrome.
      if (name === 'Normal' || name === 'Abnormal' || name === 'Status' || name.length === 0) continue;
      // Deduplicate: keep first occurrence.
      if (!(name in checks)) {
        checks[name] = status;
        if (status === 'Normal') normalCount++;
        else abnormalCount++;
      }
    }
  }

  // Top verdict: text after "Test Results:" or "Bot Detection" heading.
  var verdict = '';
  var verdictIdx = text.indexOf('Test Results:');
  if (verdictIdx >= 0) {
    var verdictEnd = text.indexOf('\n', verdictIdx);
    if (verdictEnd < 0) verdictEnd = Math.min(verdictIdx + 100, text.length);
    verdict = text.slice(verdictIdx, verdictEnd).replace('Test Results:', '').trim();
  }
  // Fallback: scan for known verdict strings.
  if (!verdict) {
    var knownVerdicts = ['No bots detected', 'Robot', 'Bot detected'];
    for (var j = 0; j < knownVerdicts.length; j++) {
      if (text.indexOf(knownVerdicts[j]) >= 0) { verdict = knownVerdicts[j]; break; }
    }
  }

  if (Object.keys(checks).length === 0 && !verdict) return null;

  return JSON.stringify({ verdict: verdict, checks: checks, normal: normalCount, abnormal: abnormalCount });
}
`

// browserscanData is the parsed result from the extract JS.
type browserscanData struct {
	Verdict  string            `json:"verdict"`
	Checks   map[string]string `json:"checks"`
	Normal   int               `json:"normal"`
	Abnormal int               `json:"abnormal"`
}

// extractBrowserScan parses results from https://www.browserscan.net/bot-detection
//
// Strategy: wait up to 20 s for "Normal" to appear in the page text (checks render
// asynchronously after ~15 s), then extract via JS.
func extractBrowserScan(ctx context.Context, page *rod.Page) (TargetResult, error) {
	result := TargetResult{
		Target: "browserscan",
		URL:    "https://www.browserscan.net/bot-detection",
	}

	// Wait for at least one "Normal" result to appear.
	deadline := time.Now().Add(browserscanWaitTimeout)
	for time.Now().Before(deadline) {
		val, err := page.Eval(browserscanReadyJS)
		if err == nil && val != nil && !isNullResult(val.Value.String()) {
			break
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("browserscan: context cancelled while waiting for results")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Extra settle: additional checks may still be writing their status.
	select {
	case <-ctx.Done():
		return result, fmt.Errorf("browserscan: context cancelled")
	case <-time.After(1 * time.Second):
	}

	val, err := page.Eval(browserscanExtractJS)
	if err != nil {
		return result, fmt.Errorf("browserscan: eval extract: %w", err)
	}
	if val == nil || isNullResult(val.Value.String()) {
		return result, fmt.Errorf("browserscan: no checks found — page structure may have changed")
	}

	var data browserscanData
	if err := parseJSON(val.Value.String(), &data); err != nil {
		return result, fmt.Errorf("browserscan: parse result: %w", err)
	}

	total := data.Normal + data.Abnormal
	var score float64
	if total > 0 {
		score = float64(data.Normal) / float64(total) * maxTrustScore
	}

	// Convert checks to map[string]any for Sections.
	checksAny := make(map[string]any, len(data.Checks))
	for k, v := range data.Checks {
		checksAny[k] = v
	}

	result.OK = true
	result.TrustScore = score
	result.Sections = map[string]any{
		"verdict":  data.Verdict,
		"checks":   checksAny,
		"normal":   data.Normal,
		"abnormal": data.Abnormal,
	}

	// Flag if verdict indicates bot detection.
	verdict := strings.ToLower(data.Verdict)
	if strings.Contains(verdict, "robot") || strings.Contains(verdict, "bot detected") {
		result.Lies = append(result.Lies, fmt.Sprintf("verdict: %s", data.Verdict))
	}

	return result, nil
}
