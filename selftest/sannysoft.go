package selftest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const sannysoftWaitTimeout = 30 * time.Second

// sannysoftExtractJS reads the results table from bot.sannysoft.com.
// Each row has: td[0]=check name, td[1]=result value, with class "passed"/"failed".
const sannysoftExtractJS = `
() => {
  var rows = document.querySelectorAll('table tr');
  if (!rows || rows.length === 0) return null;
  var results = { checks: [], passed: 0, failed: 0 };
  for (var i = 0; i < rows.length; i++) {
    var row = rows[i];
    var cells = row.querySelectorAll('td');
    if (cells.length < 2) continue;
    var name = cells[0].textContent.trim();
    var valueCell = cells[1];
    var value = valueCell.textContent.trim();
    var className = valueCell.getAttribute('class') || '';
    var ok = className.indexOf('passed') >= 0 ||
              className.indexOf('green') >= 0 ||
              value.toLowerCase() === 'present' ||
              value.toLowerCase() === 'true';
    var fail = className.indexOf('failed') >= 0 ||
               className.indexOf('red') >= 0 ||
               value.toLowerCase() === 'missing' ||
               value.toLowerCase() === 'false';
    if (name) {
      results.checks.push({ name: name, value: value, ok: ok });
      if (ok) results.passed++;
      else if (fail) results.failed++;
    }
  }
  return JSON.stringify(results);
}
`

// sannysoftReadyJS returns non-null when the results table is populated.
const sannysoftReadyJS = `
() => {
  var rows = document.querySelectorAll('table tr td');
  return rows.length > 0 ? String(rows.length) : null;
}
`

// sannysoftChecks is the parsed result from the sannysoft table.
type sannysoftChecks struct {
	Checks []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		OK    bool   `json:"ok"`
	} `json:"checks"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

// extractSannysoft parses the pass/fail table at https://bot.sannysoft.com/
//
// Strategy: wait up to 30 s for the results table to populate, then eval JS to
// read check names and pass/fail classes.
func extractSannysoft(ctx context.Context, page *rod.Page) (TargetResult, error) {
	result := TargetResult{
		Target: "sannysoft",
		URL:    "https://bot.sannysoft.com/",
	}

	// Wait for table to populate.
	deadline := time.Now().Add(sannysoftWaitTimeout)
	for time.Now().Before(deadline) {
		val, err := page.Eval(sannysoftReadyJS)
		if err == nil && val != nil && strings.TrimSpace(val.Value.String()) != "" {
			break
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("sannysoft: context cancelled while waiting for table")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Give JS a moment to finish rendering all rows.
	select {
	case <-ctx.Done():
		return result, fmt.Errorf("sannysoft: context cancelled")
	case <-time.After(1 * time.Second):
	}

	val, err := page.Eval(sannysoftExtractJS)
	if err != nil {
		return result, fmt.Errorf("sannysoft: eval extract: %w", err)
	}
	if val == nil || val.Value.String() == "" || val.Value.String() == "null" {
		return result, fmt.Errorf("sannysoft: selector not found — page may have changed structure")
	}

	var checks sannysoftChecks
	if err := parseJSON(val.Value.String(), &checks); err != nil {
		return result, fmt.Errorf("sannysoft: parse result: %w", err)
	}

	// Build sections: individual check results + summary counts.
	checkMap := make(map[string]any, len(checks.Checks))
	for _, c := range checks.Checks {
		checkMap[c.Name] = map[string]any{"value": c.Value, "ok": c.OK}
	}

	total := checks.Passed + checks.Failed
	var score float64
	if total > 0 {
		score = float64(checks.Passed) / float64(total) * maxTrustScore
	}

	result.OK = true
	result.TrustScore = score
	result.Sections = map[string]any{
		"checks": checkMap,
		"passed": checks.Passed,
		"failed": checks.Failed,
		"total":  total,
	}
	return result, nil
}

const maxTrustScore = 100.0
