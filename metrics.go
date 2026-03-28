package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anatolykoptev/go-browser/webvitals"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// CWVReport holds Core Web Vitals metrics collected from a real browser.
type CWVReport struct {
	URL     string  `json:"url"`
	FCP     float64 `json:"fcp_ms"`     // First Contentful Paint (ms)
	LCP     float64 `json:"lcp_ms"`     // Largest Contentful Paint (ms)
	CLS     float64 `json:"cls"`        // Cumulative Layout Shift (score)
	TTFB    float64 `json:"ttfb_ms"`    // Time to First Byte (ms)
	INP     float64 `json:"inp_ms"`     // Interaction to Next Paint (ms, -1 if no interaction)
	Elapsed int64   `json:"elapsed_ms"` // Total measurement time
}

// CWVGrade returns a Lighthouse-compatible rating for each metric.
func (r *CWVReport) CWVGrade() map[string]string {
	return map[string]string{
		"fcp":  gradeByThresholds(r.FCP, 1800, 3000),
		"lcp":  gradeByThresholds(r.LCP, 2500, 4000),
		"cls":  gradeByThresholds(r.CLS*1000, 100, 250), // CLS thresholds: 0.1, 0.25
		"ttfb": gradeByThresholds(r.TTFB, 800, 1800),
		"inp":  gradeByThresholds(r.INP, 200, 500),
	}
}

func gradeByThresholds(value, good, poor float64) string {
	if value < 0 {
		return "n/a"
	}
	if value <= good {
		return "good"
	}
	if value <= poor {
		return "needs-improvement"
	}
	return "poor"
}

// CollectCWV navigates to a URL in a fresh Chrome tab, injects Google's
// web-vitals.js library before page load, waits for metrics to settle,
// and returns Core Web Vitals measured by a real browser engine.
//
// This uses rod's EvalOnNewDocument to inject the library before any page
// JS runs — capturing FCP, LCP, CLS, TTFB, and INP from the very start
// of navigation, exactly as Chrome DevTools measures them.
//
// Requires a running rod.Browser instance (not the Browser interface).
func CollectCWV(ctx context.Context, b *rod.Browser, targetURL string, settleTime time.Duration) (*CWVReport, error) {
	if settleTime == 0 {
		settleTime = 3 * time.Second
	}

	start := time.Now()

	page, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	defer func() { _ = page.Close() }()

	// Inject web-vitals.js BEFORE navigation using Page.addScriptToEvaluateOnNewDocument.
	// This ensures the library loads before any page JS, capturing paint events from start.
	_, err = proto.PageAddScriptToEvaluateOnNewDocument{
		Source: webvitals.InjectionScript(),
	}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("inject web-vitals: %w", err)
	}

	// Navigate and wait for load event.
	err = page.Navigate(targetURL)
	if err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	err = page.WaitLoad()
	if err != nil {
		return nil, fmt.Errorf("wait load: %w", err)
	}

	// Settle time for CLS/LCP to finalize.
	select {
	case <-time.After(settleTime):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Force LCP/CLS finalization. web-vitals.js reports LCP on visibilitychange
	// or user input. Lighthouse also uses this technique.
	// We change visibility state via CDP to trigger proper metric finalization.
	_ = proto.PageSetLifecycleEventsEnabled{Enabled: true}.Call(page)
	_, _ = proto.RuntimeEvaluate{
		Expression: `
			Object.defineProperty(document, 'visibilityState', {value: 'hidden', configurable: true});
			document.dispatchEvent(new Event('visibilitychange'));
			Object.defineProperty(document, 'visibilityState', {value: 'visible', configurable: true});
		`,
	}.Call(page)

	// Small delay for callbacks to fire.
	time.Sleep(200 * time.Millisecond)

	// Collect metrics via CDP Runtime.evaluate (not rod's Eval which wraps in function).
	evalResult, err := proto.RuntimeEvaluate{
		Expression:    webvitals.CollectScript(),
		ReturnByValue: true,
	}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("collect metrics: %w", err)
	}
	if evalResult.ExceptionDetails != nil {
		return nil, fmt.Errorf("collect metrics JS error: %s", evalResult.ExceptionDetails.Text)
	}

	raw := evalResult.Result.Value.String()
	// Strip JSON quotes if rod/CDP wrapped the string.
	if len(raw) > 1 && raw[0] == '"' {
		var unquoted string
		if err := json.Unmarshal([]byte(raw), &unquoted); err == nil {
			raw = unquoted
		}
	}
	var cwv struct {
		FCP  float64 `json:"fcp"`
		LCP  float64 `json:"lcp"`
		CLS  float64 `json:"cls"`
		TTFB float64 `json:"ttfb"`
		INP  float64 `json:"inp"`
	}
	if err := json.Unmarshal([]byte(raw), &cwv); err != nil {
		return nil, fmt.Errorf("parse metrics: %w (raw: %s)", err, raw)
	}

	return &CWVReport{
		URL:     targetURL,
		FCP:     cwv.FCP,
		LCP:     cwv.LCP,
		CLS:     cwv.CLS,
		TTFB:    cwv.TTFB,
		INP:     cwv.INP,
		Elapsed: time.Since(start).Milliseconds(),
	}, nil
}
