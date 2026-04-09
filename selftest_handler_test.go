package browser

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-browser/selftest"
)

func TestHandleSelftest_ChromeNil(t *testing.T) {
	s := &Server{chrome: nil}

	req := httptest.NewRequest(http.MethodGet, "/selftest", nil)
	w := httptest.NewRecorder()

	s.handleSelftest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("response should contain 'error' field")
	}
}

func TestHandleSelftest_ChromeDisconnected(t *testing.T) {
	// ChromeManager with nil browser reports Connected() = false.
	chrome := &ChromeManager{}
	s := &Server{chrome: chrome}

	req := httptest.NewRequest(http.MethodGet, "/selftest", nil)
	w := httptest.NewRecorder()

	s.handleSelftest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestSelftestRouteRegistered(t *testing.T) {
	// Verify /selftest is in the mux without starting a real server.
	mux := http.NewServeMux()
	s := &Server{mux: mux, chrome: nil}
	s.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/selftest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// With chrome=nil we expect 503 (not 404 which would mean route missing).
	if w.Code == http.StatusNotFound {
		t.Error("/selftest route is not registered (got 404)")
	}
}

func TestMakePageFactory_InvalidProfile(t *testing.T) {
	// makePageFactory with a non-existent profile name should return an error
	// from LoadProfile before touching chrome.
	chrome := &ChromeManager{} // nil browser, won't connect
	factory := makePageFactory(chrome, "nonexistent_profile_xyz")

	page, cleanup, err := factory("")
	if err == nil {
		// If no error, clean up to avoid leaks.
		if cleanup != nil {
			cleanup()
		}
		if page != nil {
			_ = page.Close()
		}
		t.Fatal("expected error for nonexistent profile, got nil")
	}
}

func TestSelftestReportShape(t *testing.T) {
	// Verify the Report JSON shape matches the spec.
	r := selftest.Report{
		Profile: "mac_chrome145",
		Results: []selftest.TargetResult{
			{
				Target:     "creepjs",
				URL:        "https://abrahamjuliot.github.io/creepjs/",
				DurationMs: 1234,
				OK:         true,
				TrustScore: 95.5,
			},
		},
		Summary: selftest.Summary{
			Total:        1,
			Passed:       1,
			Failed:       0,
			OverallTrust: 95.5,
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"profile", "started_at", "results", "summary"} {
		if _, ok := m[key]; !ok {
			t.Errorf("report JSON missing key %q", key)
		}
	}

	summary, ok := m["summary"].(map[string]any)
	if !ok {
		t.Fatal("summary is not an object")
	}
	for _, key := range []string{"total", "passed", "failed", "overall_trust"} {
		if _, ok := summary[key]; !ok {
			t.Errorf("summary JSON missing key %q", key)
		}
	}
}
