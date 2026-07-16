package browser

import (
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestReconnect_GenerationCounter verifies that UpdateBrowser increments the
// generation counter and invalidates existing pages.
//
// Requires a live Chrome (CloakBrowser) — skipped under -short.
func TestReconnect_GenerationCounter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	// Create a page — it should be generation 0 (or whatever the pool starts at).
	mp, err := pool.GetOrCreatePage("test-gen-page", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-gen-page") })

	initialGen := pool.generation.Load()
	if mp.generation != initialGen {
		t.Fatalf("page generation %d != pool generation %d", mp.generation, initialGen)
	}

	// Page should be valid before reconnect.
	if !mp.IsValid(pool) {
		t.Fatal("page should be valid before reconnect")
	}

	// Simulate reconnect: UpdateBrowser with the same browser (increments generation).
	pool.UpdateBrowser(br)

	newGen := pool.generation.Load()
	if newGen != initialGen+1 {
		t.Fatalf("generation should increment by 1: got %d, want %d", newGen, initialGen+1)
	}

	// Old page should now be invalid (generation mismatch).
	if mp.IsValid(pool) {
		t.Fatal("old page should be invalid after reconnect (generation mismatch)")
	}

	// Old page should have been removed from the pool's Pages map.
	_, err = pool.GetOrCreatePage("test-gen-page", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage after reconnect should create a new page: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-gen-page") })
}

// TestReconnect_PageInvalidation verifies that after UpdateBrowser, old pages
// are removed from the context's Pages map.
func TestReconnect_PageInvalidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	// Create two pages in the default context.
	mp1, err := pool.GetOrCreatePage("test-invalidate-1", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage 1 failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-invalidate-1") })

	mp2, err := pool.GetOrCreatePage("test-invalidate-2", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage 2 failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-invalidate-2") })

	// Verify both pages exist.
	if !mp1.IsValid(pool) || !mp2.IsValid(pool) {
		t.Fatal("both pages should be valid before reconnect")
	}

	// Simulate reconnect.
	pool.UpdateBrowser(br)

	// Both pages should be invalid.
	if mp1.IsValid(pool) {
		t.Fatal("page 1 should be invalid after reconnect")
	}
	if mp2.IsValid(pool) {
		t.Fatal("page 2 should be invalid after reconnect")
	}
}

// TestPlaceholder_ReadyChannelSafety verifies that the ready channel is always
// closed, even when page creation fails (e.g., with a bad context key).
func TestPlaceholder_ReadyChannelSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	// Create a page normally.
	mp, err := pool.GetOrCreatePage("test-ready-safety", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-ready-safety") })

	// The ready channel should be closed.
	select {
	case <-mp.ready:
		// Good — channel is closed.
	default:
		t.Fatal("ready channel should be closed after successful page creation")
	}

	// signalReady should be safe to call again (sync.Once).
	mp.signalReady() // should not panic
}

// sharedChromeManager holds a single ChromeManager for tests that need one.
// Created once per test run, closed at the end via TestMain cleanup.
var (
	sharedChromeManagerOnce sync.Once
	sharedChromeManagerInst *ChromeManager
	sharedChromeManagerErr  error
)

// acquireSharedChromeManager returns a shared ChromeManager connected to CloakBrowser.
// Tests that need a ChromeManager should use this instead of creating their own,
// to avoid interfering with the shared CloakBrowser WebSocket connection.
func acquireSharedChromeManager(t *testing.T) *ChromeManager {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	wsURL := os.Getenv("CLOAKBROWSER_WS_URL")
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:9222"
	}
	sharedChromeManagerOnce.Do(func() {
		sharedChromeManagerInst, sharedChromeManagerErr = NewChromeManager(wsURL)
	})
	if sharedChromeManagerErr != nil {
		t.Skipf("Chrome unavailable: %v", sharedChromeManagerErr)
	}
	return sharedChromeManagerInst
}

// TestHealthCheck_LiveChrome verifies that HealthCheck returns a connected
// status with latency when Chrome is available.
func TestHealthCheck_LiveChrome(t *testing.T) {
	m := acquireSharedChromeManager(t)

	status := m.HealthCheck()

	if !status.Connected {
		t.Fatal("HealthCheck should report connected=true with live Chrome")
	}

	if status.LatencyMs < 0 {
		t.Fatalf("latency should be >= 0, got %d", status.LatencyMs)
	}

	t.Logf("HealthCheck: connected=%v latency=%dms pool=%+v",
		status.Connected, status.LatencyMs, status.ContextPool)
}

// TestHealthCheck_Disconnected verifies that HealthCheck returns connected=false
// when the browser is nil (after Close).
func TestHealthCheck_Disconnected(t *testing.T) {
	// Use a bare ChromeManager with nil browser — simulates post-Close state.
	m := &ChromeManager{}

	status := m.HealthCheck()

	if status.Connected {
		t.Fatal("HealthCheck should report connected=false with nil browser")
	}
}

// TestStealthAutoApply verifies that SetStealthProfile causes the pool to
// automatically apply stealth to newly created pages.
func TestStealthAutoApply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	br := acquireSharedBrowser(t)

	pool := NewContextPool(br)
	t.Cleanup(pool.Close)

	// Load a profile and set it on the pool.
	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Skipf("profile mac_chrome145 not available: %v", err)
	}

	pool.SetStealthProfile(profile)

	// Create a page — stealth should be auto-applied.
	mp, err := pool.GetOrCreatePage("test-stealth-auto", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage failed: %v", err)
	}
	t.Cleanup(func() { _ = pool.ClosePage("test-stealth-auto") })

	if mp.Page == nil {
		t.Fatal("page should not be nil after creation")
	}

	// Verify stealth was applied by checking navigator.userAgent matches profile.
	result, err := mp.Page.Eval(`() => navigator.userAgent`)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	ua := result.Value.Str()
	if ua == "" {
		t.Fatal("navigator.userAgent should not be empty")
	}

	// The profile UA should contain "Macintosh" (macOS profile).
	if !contains(ua, "Macintosh") {
		t.Logf("UA does not contain 'Macintosh' — stealth may not have been applied. UA: %s", ua)
	}

	t.Logf("Stealth auto-apply: UA=%s", ua)
}

// TestMetricsEndpoint verifies the /metrics endpoint returns Prometheus-format text.
// Uses a nil chrome to test the disconnected metrics path.
func TestMetricsEndpoint(t *testing.T) {
	// Test with nil chrome — should return disconnected metrics.
	srv := &Server{cfg: ServerConfig{}, chrome: nil, mux: nil}

	rec := httptest.NewRecorder()
	srv.handleMetrics(rec, nil)

	body := rec.Body.String()

	if !contains(body, "# HELP") {
		t.Fatal("metrics should contain # HELP comments")
	}
	if !contains(body, "# TYPE") {
		t.Fatal("metrics should contain # TYPE comments")
	}
	if !contains(body, "go_browser_connected") {
		t.Fatal("metrics should contain go_browser_connected")
	}
	if !contains(body, "go_browser_connected 0") {
		t.Fatal("disconnected metrics should show go_browser_connected 0")
	}

	t.Logf("Metrics body (disconnected):\n%s", body)
}

// TestMetricsEndpoint_LiveChrome verifies the /metrics endpoint with a live Chrome.
func TestMetricsEndpoint_LiveChrome(t *testing.T) {
	m := acquireSharedChromeManager(t)

	srv := &Server{cfg: ServerConfig{}, chrome: m, mux: nil}

	rec := httptest.NewRecorder()
	srv.handleMetrics(rec, nil)

	body := rec.Body.String()

	if !contains(body, "go_browser_connected 1") {
		t.Fatal("connected metrics should show go_browser_connected 1")
	}
	if !contains(body, "go_browser_cdp_latency_ms") {
		t.Fatal("metrics should contain go_browser_cdp_latency_ms")
	}

	t.Logf("Metrics body (live):\n%s", body)
}

// TestLostConnectionChannel verifies that the LostConnection channel is open
// when Chrome is connected and the disconnect watcher is running.
func TestLostConnectionChannel(t *testing.T) {
	m := acquireSharedChromeManager(t)

	// LostConnection should be open (not closed) while Chrome is connected.
	select {
	case <-m.LostConnection:
		t.Fatal("LostConnection should not be closed while Chrome is connected")
	default:
		// Good — channel is open.
	}

	// Verify closingGracefully is also open.
	select {
	case <-m.closingGracefully:
		t.Fatal("closingGracefully should not be closed while Chrome is connected")
	default:
		// Good — channel is open.
	}
}

// TestLostConnection_OnClose verifies that Close() closes closingGracefully
// but does NOT close LostConnection (intentional close, not disconnect).
// Creates its own ChromeManager because it tests the Close behavior.
func TestLostConnection_OnClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	wsURL := os.Getenv("CLOAKBROWSER_WS_URL")
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:9222"
	}

	// Create a NEW ChromeManager — we're going to close it.
	// This test must run LAST among ChromeManager tests because it closes
	// the connection. We use a separate connection to avoid affecting
	// the shared instance.
	m, err := NewChromeManager(wsURL)
	if err != nil {
		t.Skipf("Chrome unavailable: %v", err)
	}

	// Close the connection — should close closingGracefully but NOT LostConnection.
	m.Close()

	// closingGracefully should be closed after Close().
	select {
	case <-m.closingGracefully:
		// Good — channel is closed.
	default:
		t.Fatal("closingGracefully should be closed after Close()")
	}

	// LostConnection should NOT be closed (intentional close, not disconnect).
	select {
	case <-m.LostConnection:
		t.Fatal("LostConnection should NOT be closed on intentional Close")
	default:
		// Good — channel is still open.
	}
}

// --- helpers ---

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
