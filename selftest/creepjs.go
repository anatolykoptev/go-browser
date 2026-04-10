package selftest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

const creepJSWaitTimeout = 30 * time.Second

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
	Debug string `json:"_debug,omitempty"`
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

  // If no explicit score element, try scanning visible text for "trust score: N"
  if (!score) {
    var bodyText = document.body ? document.body.innerText : '';
    var m = bodyText.match(/trust score[:\s]*(\d{1,3})/i);
    if (m) score = parseFloat(m[1]);
    if (!score) return null;
  }

  var result = { trustScore: score, lies: [], fonts: {}, webrtc: {}, audio: {}, voices: {}, ua: {},
                 _debug: document.title + ' | ' + (document.body ? document.body.innerText.substring(0, 200) : '') };

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
  // Fallback: any element whose text looks like a trust percentage
  var els = document.querySelectorAll('span, div, p');
  for (var i = 0; i < els.length; i++) {
    var t = els[i].textContent.trim();
    if (/^trust score[:\s]*\d/i.test(t) || /^\d{2,3}%?\s*(trusted|bot|lie)/i.test(t)) return t;
  }
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

	result.OK = true
	result.TrustScore = cr.TrustScore
	result.Lies = cr.Lies
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
	return result, nil
}
