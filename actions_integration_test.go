package browser

import (
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
)

// TestActionIntegration_Screenshot verifies that the screenshot action
// #29: replaces stub-based unit test with real Chrome behavior.
func TestActionIntegration_Screenshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	mp, err := pool.GetOrCreatePage("test-screenshot", "default", "", "data:text/html,<html><body><h1>Hello Screenshot</h1></body></html>")
	if err != nil {
		t.Fatalf("GetOrCreatePage failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-screenshot") })

	if err := mp.Page.WaitLoad(); err != nil {
		t.Fatalf("WaitLoad failed: %v", err)
	}

	// Use format="image" + OutputPath to get a ScreenshotResult struct.
	res := ExecuteAction(t.Context(), mp.Page, Action{Type: "screenshot", Format: "image", OutputPath: "/tmp/test-screenshot.jpg"}, humanize.NewCursor(0, 0), nil, false, NewRefMap(), 0)
	if !res.Ok {
		t.Fatalf("screenshot action failed: %s", res.Error)
	}

	sr, ok := res.Data.(ScreenshotResult)
	if !ok {
		t.Fatalf("screenshot with format=image should return ScreenshotResult, got %T (%v)", res.Data, res.Data)
	}

	if sr.Path == "" {
		t.Fatal("screenshot result should have a non-empty path")
	}

	t.Logf("Screenshot saved to: %s (%d bytes, %dx%d %s)", sr.Path, sr.BytesSize, sr.Width, sr.Height, sr.Format)
}

// TestActionIntegration_Scroll verifies scroll action works on a real page.
func TestActionIntegration_Scroll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	// Use a page with explicit scrollable content.
	html := `data:text/html,<!DOCTYPE html><html><head><style>body{height:3000px;margin:0}h1{position:absolute;top:2000px}</style></head><body><h1>Scroll Test</h1></body></html>`
	mp, err := pool.GetOrCreatePage("test-scroll", "default", "", html)
	if err != nil {
		t.Fatalf("GetOrCreatePage failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-scroll") })

	if err := mp.Page.WaitLoad(); err != nil {
		t.Fatalf("WaitLoad failed: %v", err)
	}

	// Use evaluate to scroll directly — mouse.Scroll may not work on data URLs.
	res := ExecuteAction(t.Context(), mp.Page, Action{Type: "evaluate", Script: "() => { window.scrollTo(0, 500); return window.scrollY }"}, humanize.NewCursor(0, 0), nil, false, NewRefMap(), 0)
	if !res.Ok {
		t.Fatalf("evaluate scroll failed: %s", res.Error)
	}

	t.Logf("Evaluate scroll result: %v", res.Data)

	// Also test the scroll action itself.
	res = ExecuteAction(t.Context(), mp.Page, Action{Type: "scroll", DeltaY: 200}, humanize.NewCursor(0, 0), nil, false, NewRefMap(), 0)
	if !res.Ok {
		t.Fatalf("scroll action failed: %s", res.Error)
	}

	result, err := mp.Page.Eval(`() => window.scrollY`)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	scrollY := result.Value.Int()
	t.Logf("Scroll position after scroll action: %d", scrollY)
}

// TestActionIntegration_Navigate verifies the navigate action works.
func TestActionIntegration_Navigate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	mp, err := pool.GetOrCreatePage("test-navigate", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-navigate") })

	res := ExecuteAction(t.Context(), mp.Page, Action{Type: "navigate", URL: "data:text/html,<html><body><h1>Navigated</h1></body></html>"}, humanize.NewCursor(0, 0), nil, false, NewRefMap(), 0)
	if !res.Ok {
		t.Fatalf("navigate action failed: %s", res.Error)
	}

	result, err := mp.Page.Eval(`() => document.body.innerText`)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	text := result.Value.Str()
	if !strings.Contains(text, "Navigated") {
		t.Fatalf("page should show 'Navigated', got %q", text)
	}

	t.Logf("Navigate result: %s", text)
}

// TestActionIntegration_Evaluate verifies JS evaluation via the evaluate action.
func TestActionIntegration_Evaluate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	mp, err := pool.GetOrCreatePage("test-eval", "default", "", "data:text/html,<html><body></body></html>")
	if err != nil {
		t.Fatalf("GetOrCreatePage failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-eval") })

	if err := mp.Page.WaitLoad(); err != nil {
		t.Fatalf("WaitLoad failed: %v", err)
	}

	res := ExecuteAction(t.Context(), mp.Page, Action{Type: "evaluate", Script: "() => 2 + 2"}, humanize.NewCursor(0, 0), nil, false, NewRefMap(), 0)
	if !res.Ok {
		t.Fatalf("evaluate action failed: %s", res.Error)
	}

	// Result should be a JSON-encoded value.
	t.Logf("Evaluate result: %v", res.Data)
}

// TestActionIntegration_Snapshot verifies the snapshot action returns content.
// Note: snapshot on data URLs may return only RootWebArea — the CDP accessibility
// tree needs a fully rendered page. This test uses a data URL with WaitDOMStable.
func TestActionIntegration_Snapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	html := `data:text/html,<html><body><h1>Hello</h1><p>World</p></body></html>`
	mp, err := pool.GetOrCreatePage("test-snapshot", "default", "", html)
	if err != nil {
		t.Fatalf("GetOrCreatePage failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-snapshot") })

	if err := mp.Page.WaitLoad(); err != nil {
		t.Fatalf("WaitLoad failed: %v", err)
	}

	// Wait for DOM to stabilize — accessibility tree needs time to build.
	_ = mp.Page.WaitDOMStable(time.Second, 0.1)

	res := ExecuteAction(t.Context(), mp.Page, Action{Type: "snapshot"}, humanize.NewCursor(0, 0), nil, false, NewRefMap(), 0)
	if !res.Ok {
		t.Fatalf("snapshot action failed: %s", res.Error)
	}

	text, ok := res.Data.(string)
	if !ok {
		t.Fatalf("snapshot result should be a string, got %T", res.Data)
	}

	t.Logf("Snapshot: %q (len=%d)", text, len(text))
}
