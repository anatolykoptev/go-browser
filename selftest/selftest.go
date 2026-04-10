// Package selftest runs live antibot probe pages through CloakBrowser and
// returns a structured trust report. Use the /selftest HTTP endpoint to invoke.
package selftest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-rod/rod"
)

const (
	// maxConcurrent caps parallel page opens against CloakBrowser.
	maxConcurrent = 3
	// perTargetTimeout is the total per-target budget (navigation + extraction).
	// Heavy antibot pages (creepjs, browserleaks) can take 20-25 s to load alone.
	perTargetTimeout = 60 * time.Second
	// pageNavTimeout is the rod page-level timeout for Navigate + WaitLoad.
	pageNavTimeout = 25 * time.Second
	// overallTimeout caps the entire /selftest run.
	overallTimeout = 120 * time.Second
	// screenshotDir is where per-run screenshots are saved.
	screenshotDir = "/tmp/selftest"
)

// PageFactory creates a new stealth page for a target run.
// The returned page should already have a browser context and stealth patches applied.
// The caller is responsible for closing the page and disposing the browser context.
// The factory receives the profile name (empty = default profile).
type PageFactory func(profile string) (*rod.Page, func(), error)

// Run executes the given targets (or all if targets is empty) and returns a Report.
// factory is called once per target to produce an isolated stealth page.
// If screenshot is true, a full-page PNG is saved under /tmp/selftest/ per target.
func Run(ctx context.Context, factory PageFactory, targets []string, profile string, screenshot bool) (Report, error) {
	ctx, cancel := context.WithTimeout(ctx, overallTimeout)
	defer cancel()

	toRun := resolveTargets(targets)
	results := make([]TargetResult, len(toRun))

	// sem limits concurrent page opens to maxConcurrent.
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, t := range toRun {
		i, t := i, t
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = runTarget(ctx, factory, t, profile, screenshot)
		}()
	}

	wg.Wait()

	report := Report{
		Profile:   profile,
		StartedAt: time.Now().UTC(),
		Results:   results,
		Summary:   buildSummary(results),
	}
	return report, nil
}

// runTarget navigates to one target URL and runs its extractor.
func runTarget(ctx context.Context, factory PageFactory, t Target, profile string, screenshot bool) TargetResult {
	result := TargetResult{Target: t.Key, URL: t.URL}
	start := time.Now()

	tCtx, cancel := context.WithTimeout(ctx, perTargetTimeout)
	defer cancel()

	page, cleanup, err := factory(profile)
	if err != nil {
		result.Error = fmt.Sprintf("create page: %s", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	defer cleanup()

	// pageNavTimeout caps navigate+WaitLoad; the extractor gets the remaining budget.
	navPage := page.Timeout(pageNavTimeout)

	if err := navPage.Navigate(t.URL); err != nil {
		result.Error = fmt.Sprintf("navigate: %s", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	if err := navPage.WaitLoad(); err != nil {
		result.Error = fmt.Sprintf("wait load: %s", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	extractor, ok := Extractors[t.Key]
	if !ok {
		result.Error = fmt.Sprintf("no extractor registered for target %q", t.Key)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	extracted, err := extractor(tCtx, page)
	if err != nil {
		result.Error = err.Error()
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	// Preserve navigation metadata even if extractor doesn't set them.
	extracted.Target = t.Key
	extracted.URL = t.URL
	extracted.DurationMs = time.Since(start).Milliseconds()

	if screenshot {
		extracted.ScreenshotPath = saveScreenshot(page, t.Key)
	}

	return extracted
}

// resolveTargets returns the Target list for the given keys, or AllTargets if empty.
func resolveTargets(keys []string) []Target {
	if len(keys) == 0 {
		return AllTargets
	}
	byKey := make(map[string]Target, len(AllTargets))
	for _, t := range AllTargets {
		byKey[t.Key] = t
	}
	out := make([]Target, 0, len(keys))
	for _, k := range keys {
		if t, ok := byKey[k]; ok {
			out = append(out, t)
		}
	}
	return out
}

// buildSummary computes aggregate stats from per-target results.
func buildSummary(results []TargetResult) Summary {
	s := Summary{Total: len(results)}
	var totalTrust float64
	var trustCount int
	for _, r := range results {
		if r.OK {
			s.Passed++
			if r.TrustScore > 0 {
				totalTrust += r.TrustScore
				trustCount++
			}
		} else {
			s.Failed++
		}
	}
	if trustCount > 0 {
		s.OverallTrust = totalTrust / float64(trustCount)
	}
	return s
}

// saveScreenshot takes a full-page PNG and writes it to screenshotDir.
// Returns the path on success, empty string on failure (non-fatal).
func saveScreenshot(page *rod.Page, key string) string {
	if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
		return ""
	}
	ts := time.Now().UTC().Format("20060102T150405")
	path := filepath.Join(screenshotDir, fmt.Sprintf("%s-%s.png", key, ts))
	data, err := page.Screenshot(true, nil)
	if err != nil {
		return ""
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // world-readable screenshot OK
		return ""
	}
	return path
}

// parseJSON unmarshals raw JSON into v.
func parseJSON(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}

// isNullResult reports whether a JS eval result is null/undefined/empty.
// rod represents JS null as the Go string "<nil>" (via gson.JSON.String() → fmt.Sprintf("%v", nil)).
func isNullResult(s string) bool {
	return s == "" || s == "null" || s == "<nil>" || s == "undefined"
}
