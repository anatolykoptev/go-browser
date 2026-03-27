package browser

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInteractRequest_Parse(t *testing.T) {
	proxy := "http://proxy.example.com:8080"
	sessionID := "new"

	raw := `{
		"url": "https://example.com",
		"actions": [
			{"type": "click", "selector": "#btn"},
			{"type": "screenshot"}
		],
		"timeout_secs": 60,
		"proxy": "http://proxy.example.com:8080",
		"session_id": "new"
	}`

	var req InteractRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.URL != "https://example.com" {
		t.Errorf("URL: got %q, want %q", req.URL, "https://example.com")
	}
	if len(req.Actions) != 2 {
		t.Fatalf("Actions len: got %d, want 2", len(req.Actions))
	}
	if req.Actions[0].Type != "click" || req.Actions[0].Selector != "#btn" {
		t.Errorf("Actions[0]: got %+v", req.Actions[0])
	}
	if req.Actions[1].Type != "screenshot" {
		t.Errorf("Actions[1]: got %+v", req.Actions[1])
	}
	if req.TimeoutSecs != 60 {
		t.Errorf("TimeoutSecs: got %d, want 60", req.TimeoutSecs)
	}
	if req.Proxy == nil || *req.Proxy != proxy {
		t.Errorf("Proxy: got %v, want %q", req.Proxy, proxy)
	}
	if req.SessionID == nil || *req.SessionID != sessionID {
		t.Errorf("SessionID: got %v, want %q", req.SessionID, sessionID)
	}
}

func TestInteractRequest_Parse_Minimal(t *testing.T) {
	raw := `{"url": "https://example.com"}`

	var req InteractRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.URL != "https://example.com" {
		t.Errorf("URL: got %q", req.URL)
	}
	if req.Proxy != nil {
		t.Errorf("Proxy should be nil for minimal request")
	}
	if req.SessionID != nil {
		t.Errorf("SessionID should be nil for minimal request")
	}
	if req.TimeoutSecs != 0 {
		t.Errorf("TimeoutSecs: got %d, want 0 (default)", req.TimeoutSecs)
	}
}

func TestInteractResponse_JSON(t *testing.T) {
	resp := InteractResponse{
		URL:    "https://example.com/final",
		Status: "ok",
		Actions: []ActionResult{
			{Action: "click", Ok: true},
			{Action: "screenshot", Ok: true, Data: "base64data"},
		},
		SessionID: "abc123",
		ElapsedMs: 1234,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(b)
	for _, want := range []string{
		`"url":"https://example.com/final"`,
		`"status":"ok"`,
		`"session_id":"abc123"`,
		`"elapsed_ms":1234`,
		`"action":"click"`,
		`"ok":true`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in JSON: %s", want, s)
		}
	}
}

func TestInteractResponse_JSON_ErrorOmitsEmptyFields(t *testing.T) {
	resp := InteractResponse{
		URL:       "https://example.com",
		Status:    "error",
		Actions:   []ActionResult{},
		Error:     "navigation failed",
		ElapsedMs: 500,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(b)
	if strings.Contains(s, `"session_id"`) {
		t.Errorf("session_id should be omitted when empty: %s", s)
	}
	if !strings.Contains(s, `"error":"navigation failed"`) {
		t.Errorf("error field missing: %s", s)
	}
}
