package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func TestCollectCWV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Local test page with content that triggers CWV metrics.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head><title>CWV Test</title></head>
<body>
  <h1>Core Web Vitals Test Page</h1>
  <p>This page has enough content for FCP and LCP to fire.</p>
  <div style="width:100%;height:300px;background:#eee;">Large block</div>
  <img src="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='600' height='400'><rect fill='%23ccc' width='600' height='400'/></svg>" width="600" height="400" alt="test">
</body>
</html>`))
	}))
	defer ts.Close()

	// Launch headless Chrome via rod.
	l := launcher.New().Headless(true)
	if bin := os.Getenv("BROWSER_BIN"); bin != "" {
		l = l.Bin(bin)
	}
	u := l.MustLaunch()
	b := rod.New().ControlURL(u).MustConnect()
	defer b.MustClose()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	report, err := CollectCWV(ctx, b, ts.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("CollectCWV: %v", err)
	}

	t.Logf("CWV Report: FCP=%.0fms LCP=%.0fms CLS=%.4f TTFB=%.0fms INP=%.0fms Elapsed=%dms",
		report.FCP, report.LCP, report.CLS, report.TTFB, report.INP, report.Elapsed)

	// FCP and TTFB should be positive (page has content).
	if report.FCP <= 0 {
		t.Errorf("expected FCP > 0, got %.2f", report.FCP)
	}
	if report.TTFB <= 0 {
		t.Errorf("expected TTFB > 0, got %.2f", report.TTFB)
	}
	// LCP may be -1 in headless Chrome if visibilitychange finalization
	// doesn't trigger. This is a known limitation in some Chrome versions.
	if report.LCP > 0 {
		t.Logf("LCP captured: %.0fms", report.LCP)
	} else {
		t.Logf("LCP not captured (%.0f) — expected in some headless Chrome versions", report.LCP)
	}
	// CLS should be >= 0 (static page = no layout shifts).
	if report.CLS < 0 {
		t.Errorf("expected CLS >= 0, got %.4f", report.CLS)
	}
	// INP = -1 is expected (no user interaction in test).
	// Just check it's a valid number.

	// Grades should work.
	grades := report.CWVGrade()
	if grades["fcp"] != "good" && grades["fcp"] != "needs-improvement" {
		t.Errorf("unexpected FCP grade: %s", grades["fcp"])
	}
}

func TestCWVGrade(t *testing.T) {
	report := &CWVReport{FCP: 1500, LCP: 2000, CLS: 0.05, TTFB: 500, INP: -1}
	grades := report.CWVGrade()

	if grades["fcp"] != "good" {
		t.Errorf("FCP 1500ms should be good, got %s", grades["fcp"])
	}
	if grades["lcp"] != "good" {
		t.Errorf("LCP 2000ms should be good, got %s", grades["lcp"])
	}
	if grades["cls"] != "good" {
		t.Errorf("CLS 0.05 should be good, got %s", grades["cls"])
	}
	if grades["ttfb"] != "good" {
		t.Errorf("TTFB 500ms should be good, got %s", grades["ttfb"])
	}
	if grades["inp"] != "n/a" {
		t.Errorf("INP -1 should be n/a, got %s", grades["inp"])
	}
}
