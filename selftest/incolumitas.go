package selftest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// incolumitasRaw is the raw result from the extract JS.
type incolumitasRaw struct {
	NewTests        *string `json:"newTests"`
	Fpscanner       *string `json:"fpscanner"`
	Intoli          *string `json:"intoli"`
	IPInfo          *string `json:"ipInfo"`
	BehavioralScore *string `json:"behavioralScore"`
}

// extractIncolumitas parses results from https://bot.incolumitas.com/
//
// Strategy: wait up to 25 s for the new-tests JSON block to appear in the page
// text (the page runs async checks over ~20 s), then extract via JS.
func extractIncolumitas(ctx context.Context, page *rod.Page) (TargetResult, error) {
	result := TargetResult{
		Target: "incolumitas",
		URL:    "https://bot.incolumitas.com/",
	}

	// Wait for new-tests JSON to appear.
	deadline := time.Now().Add(incolumitasWaitTimeout)
	for time.Now().Before(deadline) {
		val, err := page.Eval(incolumitasReadyJS)
		if err == nil && val != nil && !isNullResult(val.Value.String()) {
			break
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("incolumitas: context cancelled while waiting for results")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Short settle — page may still be writing final JSON blocks.
	select {
	case <-ctx.Done():
		return result, fmt.Errorf("incolumitas: context cancelled")
	case <-time.After(1 * time.Second):
	}

	val, err := page.Eval(incolumitasExtractJS)
	if err != nil {
		return result, fmt.Errorf("incolumitas: eval extract: %w", err)
	}
	if val == nil || isNullResult(val.Value.String()) {
		return result, fmt.Errorf("incolumitas: no result extracted — page structure may have changed")
	}

	var raw incolumitasRaw
	if err := parseJSON(val.Value.String(), &raw); err != nil {
		return result, fmt.Errorf("incolumitas: parse raw envelope: %w", err)
	}

	newTests, newOK, newFail := parseIncolumitasChecks(raw.NewTests)
	fpscanner, fpsOK, fpsFail := parseIncolumitasFlatChecks(raw.Fpscanner)
	intoli, _, _ := parseIncolumitasFlatChecks(raw.Intoli)

	sections := map[string]any{
		"new_tests": newTests,
		"fpscanner": fpscanner,
		"intoli":    intoli,
	}
	if raw.IPInfo != nil {
		var ipInfo map[string]any
		if err := parseJSON(*raw.IPInfo, &ipInfo); err == nil {
			sections["ip_info"] = ipInfo
		}
	}
	if raw.BehavioralScore != nil {
		sections["behavioral_score"] = *raw.BehavioralScore
	}

	totalOK := newOK + fpsOK
	totalFail := newFail + fpsFail
	total := totalOK + totalFail

	var score float64
	if total > 0 {
		score = float64(totalOK) / float64(total) * maxTrustScore
	}

	result.OK = true
	result.TrustScore = score
	result.Sections = sections
	return result, nil
}

// parseIncolumitasChecks parses a JSON object where values are "OK"/"FAIL" strings.
// Returns the check map and OK/FAIL counts.
func parseIncolumitasChecks(raw *string) (map[string]any, int, int) {
	if raw == nil {
		return nil, 0, 0
	}
	var m map[string]string
	if err := parseJSON(*raw, &m); err != nil {
		return nil, 0, 0
	}
	out := make(map[string]any, len(m))
	okCount, failCount := 0, 0
	for k, v := range m {
		out[k] = v
		if strings.EqualFold(v, "OK") {
			okCount++
		} else {
			failCount++
		}
	}
	return out, okCount, failCount
}

// parseIncolumitasFlatChecks parses a JSON object counting string "OK"/"FAIL" leaves.
func parseIncolumitasFlatChecks(raw *string) (map[string]any, int, int) {
	if raw == nil {
		return nil, 0, 0
	}
	var m map[string]any
	if err := parseJSON(*raw, &m); err != nil {
		return nil, 0, 0
	}
	okCount, failCount := 0, 0
	for _, v := range m {
		if s, ok := v.(string); ok {
			if strings.EqualFold(s, "OK") {
				okCount++
			} else {
				failCount++
			}
		}
	}
	return m, okCount, failCount
}
