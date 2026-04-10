package selftest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

const creepJSWaitTimeout = 65 * time.Second

// creepJSResult is the structure extracted from window.__creepResult on creepjs.
type creepJSResult struct {
	TrustScore float64  `json:"trustScore"`
	Lies       []string `json:"lies"`
	Fonts      struct {
		Hash               string `json:"hash"`
		PlatformClassifier string `json:"platformClassifier"`
	} `json:"fonts"`
	WebRTC struct {
		PublicIP string   `json:"publicIp"`
		LocalIPs []string `json:"localIps"`
	} `json:"webrtc"`
	Audio struct {
		Hash string `json:"hash"`
	} `json:"audio"`
	Voices struct {
		Count int    `json:"count"`
		Hash  string `json:"hash"`
	} `json:"voices"`
	UA struct {
		Brands   []map[string]string `json:"brands"`
		Platform string              `json:"platform"`
	} `json:"ua"`
	Debug    string `json:"_debug,omitempty"`
	HasScore bool   `json:"_hasScore"`
}

// creepJSExtractJS is evaluated in the page to extract results from creepjs.
// creepjs populates window.__fingerprint or exposes the score in the DOM.
// We poll for the trust score element to appear then extract via the DOM.
const creepJSExtractJS = `
() => {
  // Try window.__creepResult first (internal state)
  if (window.__creepResult) return JSON.stringify(window.__creepResult);

  // Try finding trust score in the rendered DOM (selectors updated 2026)
  var scoreEl = document.querySelector(
    '#creep-results .trust-score, .trust-score, .trust, [data-trust], ' +
    '#creepjs .summary, .creepjs-trust, .fingerprint-trust'
  );
  var score = 0;
  if (scoreEl) {
    score = parseFloat(scoreEl.textContent.replace(/[^0-9.]/g, '')) || 0;
  }

  // Capture body text and debug info before deciding to fail
  var bodyText = document.body ? document.body.innerText : '';
  var debugSnippet = document.title + ' | bodyLen:' + bodyText.length + ' | tail[200]: ' + bodyText.substring(Math.max(0, bodyText.length - 200));

  // If no explicit score element, try scanning visible text for "trust score: N" or "N%"
  if (!score) {
    var m1 = bodyText.match(/trust score[:\s]*(\d{1,3})/i);
    if (m1) score = parseFloat(m1[1]);
  }
  if (!score) {
    // creepjs often renders score as "NN%" near "bot" or "lie" indicators
    var m2 = bodyText.match(/\b(\d{2,3})%?\s*(trusted|bot detected|lie)/i);
    if (m2) score = parseFloat(m2[1]);
  }
  if (!score) {
    // Last resort: look for any 2-3 digit number after "score"
    var m3 = bodyText.match(/score[^0-9]*(\d{2,3})/i);
    if (m3) score = parseFloat(m3[1]);
  }

  // Return debug info even if we can't find a score, so we can see what's on the page
  var result = { trustScore: score || 0, lies: [], fonts: {}, webrtc: {}, audio: {}, voices: {}, ua: {},
                 _debug: debugSnippet,
                 _hasScore: score > 0 };

  // Collect lies (use for loop to avoid prototype patching issues)
  var lieEls = document.querySelectorAll('.lie, .lies li, [data-lie]');
  for (var i = 0; i < lieEls.length; i++) {
    var t = lieEls[i].textContent.trim();
    if (t) result.lies.push(t);
  }

  // Fonts
  var fontEl = document.querySelector('[data-id="fonts"] .hash, .fonts-hash');
  if (fontEl) result.fonts.hash = fontEl.textContent.trim();

  // Audio
  var audioEl = document.querySelector('[data-id="audio"] .hash, .audio-hash');
  if (audioEl) result.audio.hash = audioEl.textContent.trim();

  return JSON.stringify(result);
}
`

// creepJSReadyJS returns a non-null numeric string when the trust score has been computed.
// creepjs can take 5-15 s to finish all fingerprinting phases.
const creepJSReadyJS = `
() => {
  // Look for a numeric trust score — creepjs renders "94%" or "94" when done.
  // Selectors observed in 2025-2026 DOM.
  var el = document.querySelector('.trust-score, #creep-results .score, .fingerprint-data, #creepjs .summary, .creepjs-trust');
  if (el) {
    var txt = el.textContent.trim();
    if (/\d{2,3}/.test(txt)) return txt;
  }
  // Fallback: scan full body text for trust score or lie count patterns
  var bodyText = document.body ? document.body.innerText : '';
  if (/trust score[:\s]*\d/i.test(bodyText)) return 'body-trust';
  if (/\d+ lie/i.test(bodyText) && bodyText.length > 2000) return 'body-lies';
  // Also fire when body is large enough (all modules rendered; 3000 chars = sufficient content)
  if (bodyText.length > 3000) return 'body-ready';
  return null;
}
`

// extractCreepJS extracts trust score, lies, and per-section hashes from
// https://abrahamjuliot.github.io/creepjs/
//
// Strategy: wait up to 30 s for the trust-score element, then extract via JS eval.
func extractCreepJS(ctx context.Context, page *rod.Page) (TargetResult, error) {
	result := TargetResult{
		Target: "creepjs",
		URL:    "https://abrahamjuliot.github.io/creepjs/",
	}

	// Wait for the analysis to complete (trust score element visible).
	deadline := time.Now().Add(creepJSWaitTimeout)
	var rawJSON string
	for time.Now().Before(deadline) {
		val, err := page.Eval(creepJSReadyJS)
		if err == nil && val != nil && !isNullResult(val.Value.String()) {
			break
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("creepjs: context cancelled while waiting for results")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Extract structured data.
	val, err := page.Eval(creepJSExtractJS)
	if err != nil {
		return result, fmt.Errorf("creepjs: eval extract: %w", err)
	}
	if val == nil || isNullResult(val.Value.String()) {
		return result, fmt.Errorf("creepjs: selector not found — page may have changed structure")
	}
	rawJSON = val.Value.String()

	var cr creepJSResult
	if err := json.Unmarshal([]byte(rawJSON), &cr); err != nil {
		return result, fmt.Errorf("creepjs: parse result: %w (raw: %.100s)", err, rawJSON)
	}

	sections := map[string]any{
		"fonts":  map[string]any{"hash": cr.Fonts.Hash, "platformClassifier": cr.Fonts.PlatformClassifier},
		"webrtc": map[string]any{"publicIp": cr.WebRTC.PublicIP, "localIps": cr.WebRTC.LocalIPs},
		"audio":  map[string]any{"hash": cr.Audio.Hash},
		"voices": map[string]any{"count": cr.Voices.Count, "hash": cr.Voices.Hash},
		"ua":     map[string]any{"brands": cr.UA.Brands, "platform": cr.UA.Platform},
	}
	if cr.Debug != "" {
		sections["_debug"] = cr.Debug
	}
	result.Sections = sections

	if !cr.HasScore {
		// Page loaded but no score found — return debug info without error.
		result.OK = false
		result.Error = "creepjs: trust score not found in page (see sections._debug)"
		return result, nil
	}

	result.OK = true
	result.TrustScore = cr.TrustScore
	result.Lies = cr.Lies
	return result, nil
}
