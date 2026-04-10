package selftest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const canvasWaitTimeout = 30 * time.Second

// canvasReadyJS returns non-null when the canvas fingerprint has been computed.
const canvasReadyJS = `
() => {
  var el = document.querySelector('.hash, [data-hash], #canvas-hash, .fingerprint-hash');
  if (el) return el.textContent.trim();
  // Also check for any canvas-related data in tables
  var rows = document.querySelectorAll('table tr td');
  return rows.length > 2 ? String(rows.length) : null;
}
`

// canvasExtractJS extracts canvas fingerprint hash and uniqueness from browserleaks.
const canvasExtractJS = `
() => {
  var out = { hash: '', uniqueness: '', raw: '' };

  // browserleaks.com/canvas shows hash in a prominent display element
  var hashEl = document.querySelector('.hash, #canvas-hash, [data-hash], .fp-hash');
  if (hashEl) out.hash = hashEl.textContent.trim();

  // Uniqueness percentile
  var uniqEl = document.querySelector('.uniqueness, [data-uniqueness], .percentile');
  if (uniqEl) out.uniqueness = uniqEl.textContent.trim();

  // Table-based extraction fallback
  document.querySelectorAll('table tr').forEach(function(row) {
    var cells = row.querySelectorAll('td');
    if (cells.length < 2) return;
    var label = cells[0].textContent.trim().toLowerCase();
    var value = cells[1].textContent.trim();
    if (label.includes('hash') || label.includes('fingerprint')) {
      if (!out.hash) out.hash = value;
    }
    if (label.includes('unique') || label.includes('percentile')) {
      if (!out.uniqueness) out.uniqueness = value;
    }
  });

  // Capture a broader raw section for debugging
  var section = document.querySelector('#canvas, .canvas-section, main');
  if (section) out.raw = section.textContent.substring(0, 200).trim();

  return JSON.stringify(out);
}
`

// extractCanvas extracts canvas fingerprint hash from https://browserleaks.com/canvas
//
// Strategy: wait for hash element to appear, extract hash + uniqueness.
// ok=true always (canvas fingerprint is informational; we just capture the hash).
func extractCanvas(ctx context.Context, page *rod.Page) (TargetResult, error) {
	result := TargetResult{
		Target: "canvas",
		URL:    "https://browserleaks.com/canvas",
	}

	deadline := time.Now().Add(canvasWaitTimeout)
	for time.Now().Before(deadline) {
		val, err := page.Eval(canvasReadyJS)
		if err == nil && val != nil {
			s := strings.TrimSpace(val.Value.String())
			if !isNullResult(s) {
				break
			}
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("canvas: context cancelled while waiting")
		case <-time.After(500 * time.Millisecond):
		}
	}

	val, err := page.Eval(canvasExtractJS)
	if err != nil {
		return result, fmt.Errorf("canvas: eval extract: %w", err)
	}
	if val == nil || isNullResult(val.Value.String()) {
		return result, fmt.Errorf("canvas: selector not found")
	}

	var canvasData struct {
		Hash       string `json:"hash"`
		Uniqueness string `json:"uniqueness"`
		Raw        string `json:"raw"`
	}
	if err := parseJSON(val.Value.String(), &canvasData); err != nil {
		return result, fmt.Errorf("canvas: parse result: %w", err)
	}

	// Canvas extraction is always informational — ok=true as long as we got data.
	result.OK = canvasData.Hash != ""
	result.TrustScore = maxTrustScore
	result.Sections = map[string]any{
		"hash":       canvasData.Hash,
		"uniqueness": canvasData.Uniqueness,
	}
	if !result.OK {
		result.Error = "canvas hash not found in page"
		result.TrustScore = 0
	}
	return result, nil
}
