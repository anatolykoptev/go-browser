package browser

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRenderRequest_Parse(t *testing.T) {
	raw := `{"url":"https://example.com","timeout_secs":30,"proxy":"http://proxy:8080"}`

	var req RenderRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", req.URL, "https://example.com")
	}
	if req.TimeoutSecs != 30 {
		t.Errorf("TimeoutSecs = %d, want 30", req.TimeoutSecs)
	}
	if req.Proxy != "http://proxy:8080" {
		t.Errorf("Proxy = %q, want %q", req.Proxy, "http://proxy:8080")
	}
}

func TestRenderRequest_Parse_Defaults(t *testing.T) {
	raw := `{"url":"https://example.com"}`

	var req RenderRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.TimeoutSecs != 0 {
		t.Errorf("TimeoutSecs = %d, want 0 (omitempty default)", req.TimeoutSecs)
	}
	if req.Proxy != "" {
		t.Errorf("Proxy = %q, want empty", req.Proxy)
	}
}

func TestRenderResponse_JSON(t *testing.T) {
	resp := RenderResponse{
		URL:       "https://example.com",
		HTML:      "<html><body>hello</body></html>",
		Title:     "Example",
		Status:    http.StatusOK,
		ElapsedMs: 123,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got RenderResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}

	if got.URL != resp.URL {
		t.Errorf("URL = %q, want %q", got.URL, resp.URL)
	}
	if got.HTML != resp.HTML {
		t.Errorf("HTML = %q, want %q", got.HTML, resp.HTML)
	}
	if got.Title != resp.Title {
		t.Errorf("Title = %q, want %q", got.Title, resp.Title)
	}
	if got.Status != resp.Status {
		t.Errorf("Status = %d, want %d", got.Status, resp.Status)
	}
	if got.ElapsedMs != resp.ElapsedMs {
		t.Errorf("ElapsedMs = %d, want %d", got.ElapsedMs, resp.ElapsedMs)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}
}

func TestRenderResponse_JSON_ErrorField(t *testing.T) {
	resp := RenderResponse{
		URL:   "https://example.com",
		Error: "navigation failed",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got RenderResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Error != resp.Error {
		t.Errorf("Error = %q, want %q", got.Error, resp.Error)
	}
}

func TestHandleRender_ChromeNil(t *testing.T) {
	s := &Server{chrome: nil}

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/render", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleRender(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
