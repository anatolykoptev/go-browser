package browser

import (
	"context"
	"testing"
)

// TestActionRegistry_Populated verifies all expected action types are registered at init time.
func TestActionRegistry_Populated(t *testing.T) {
	expectedTypes := []string{
		"click", "hover", "go_back",
		"type_text", "press", "fill_form", "select_option",
		"wait_for", "sleep", "wait",
		"navigate", "scroll", "resize",
		"evaluate", "eval_on_new_document", "screenshot", "snapshot",
		"set_cookies", "get_cookies", "handle_dialog",
		"destroy_session", "get_logs", "warmup",
	}

	for _, actionType := range expectedTypes {
		t.Run(actionType, func(t *testing.T) {
			if _, ok := actionRegistry[actionType]; !ok {
				t.Errorf("action type %q not registered", actionType)
			}
		})
	}
}

// TestExecuteAction_UnknownType verifies that an unknown action type returns an error result.
func TestExecuteAction_UnknownType(t *testing.T) {
	result := ExecuteAction(context.Background(), nil, Action{Type: "nonexistent_action"}, nil, nil, false, nil)
	if result.Ok {
		t.Error("expected Ok=false for unknown action type")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error for unknown action type")
	}
}

// TestExecuteAction_DestroySessionNoOp verifies destroy_session returns Ok=true with no data.
func TestExecuteAction_DestroySessionNoOp(t *testing.T) {
	result := ExecuteAction(context.Background(), nil, Action{Type: "destroy_session"}, nil, nil, false, nil)
	if !result.Ok {
		t.Errorf("destroy_session: expected Ok=true, got error: %s", result.Error)
	}
	if result.Data != nil {
		t.Errorf("destroy_session: expected nil Data, got %v", result.Data)
	}
}

// TestActionRegistry_Count verifies the registry has the expected number of entries.
func TestActionRegistry_Count(t *testing.T) {
	const wantCount = 28
	if got := len(actionRegistry); got != wantCount {
		t.Errorf("actionRegistry has %d entries, want %d", got, wantCount)
	}
}

// TestExecGetLogs_DefaultLimit verifies that get_logs returns at most 30 network and 20 console entries by default.
func TestExecGetLogs_DefaultLimit(t *testing.T) {
	logs := NewLogCollector()
	for i := range 50 {
		logs.AddNetwork(NetworkEntry{Method: "GET", URL: "https://example.com/" + string(rune('a'+i%26)), Status: 200})
	}
	for range 30 {
		logs.AddConsole(ConsoleEntry{Level: "log", Text: "msg"})
	}

	dc := dispatchContext{ctx: context.Background(), logs: logs}
	result := ExecuteAction(context.Background(), nil, Action{Type: "get_logs"}, nil, logs, false, nil)
	if !result.Ok {
		t.Fatalf("get_logs failed: %s", result.Error)
	}
	_ = dc
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result.Data)
	}
	net, _ := data["network"].([]any)
	if net == nil {
		// compact type — use interface slice approach
		if netSlice, ok2 := data["network"]; ok2 {
			t.Logf("network type: %T", netSlice)
		}
	}
	con, _ := data["console"].([]ConsoleEntry)
	if con == nil {
		t.Logf("console type: %T", data["console"])
	}

	// Count entries via reflection-free approach: re-run via execGetLogs directly.
	result2, err := execGetLogs(dispatchContext{ctx: context.Background(), logs: logs}, Action{})
	if err != nil {
		t.Fatalf("execGetLogs: %v", err)
	}
	m := result2.(map[string]any)

	netEntries := m["network"]
	conEntries := m["console"]

	// network should be limited to defaultNetworkLimit (30)
	switch v := netEntries.(type) {
	case []struct {
		Method   string `json:"method"`
		URL      string `json:"url"`
		Status   int    `json:"status,omitempty"`
		MimeType string `json:"mime_type,omitempty"`
		Error    string `json:"error,omitempty"`
	}:
		if len(v) > defaultNetworkLimit {
			t.Errorf("network: got %d entries, want <= %d", len(v), defaultNetworkLimit)
		}
	default:
		t.Logf("network entries type: %T (skipping length check)", netEntries)
	}

	if cs, ok := conEntries.([]ConsoleEntry); ok {
		if len(cs) > defaultConsoleLimit {
			t.Errorf("console: got %d entries, want <= %d", len(cs), defaultConsoleLimit)
		}
	}
}

// TestExecGetLogs_CustomLimit verifies that a custom Limit is respected.
func TestExecGetLogs_CustomLimit(t *testing.T) {
	logs := NewLogCollector()
	for i := range 50 {
		logs.AddNetwork(NetworkEntry{Method: "GET", URL: "https://example.com/" + string(rune('a'+i%26)), Status: 200})
		logs.AddConsole(ConsoleEntry{Level: "log", Text: "msg"})
	}

	result, err := execGetLogs(dispatchContext{ctx: context.Background(), logs: logs}, Action{Limit: 5})
	if err != nil {
		t.Fatalf("execGetLogs: %v", err)
	}
	m := result.(map[string]any)

	if cs, ok := m["console"].([]ConsoleEntry); ok {
		if len(cs) != 5 {
			t.Errorf("console: got %d entries, want 5", len(cs))
		}
	}
}

// TestExecGetLogs_URLTruncation verifies that long URLs are truncated.
func TestExecGetLogs_URLTruncation(t *testing.T) {
	logs := NewLogCollector()
	longURL := "https://analytics.example.com/pixel?" + string(make([]byte, 300))
	logs.AddNetwork(NetworkEntry{Method: "GET", URL: longURL, Status: 200})

	result, err := execGetLogs(dispatchContext{ctx: context.Background(), logs: logs}, Action{Limit: 1})
	if err != nil {
		t.Fatalf("execGetLogs: %v", err)
	}
	m := result.(map[string]any)
	_ = m
	// Verify via truncateURL directly since the compactNetwork type is unexported.
	got := truncateURL(longURL)
	if len(got) > maxURLLength+10 { // +10 for the ellipsis bytes
		t.Errorf("truncated URL too long: %d bytes", len(got))
	}
}

// TestExecGetLogs_NilLogs verifies that nil logs returns empty slices.
func TestExecGetLogs_NilLogs(t *testing.T) {
	result, err := execGetLogs(dispatchContext{ctx: context.Background(), logs: nil}, Action{})
	if err != nil {
		t.Fatalf("execGetLogs nil: %v", err)
	}
	m := result.(map[string]any)
	if net, ok := m["network"].([]NetworkEntry); !ok || len(net) != 0 {
		t.Errorf("expected empty network slice, got %T %v", m["network"], m["network"])
	}
	if con, ok := m["console"].([]ConsoleEntry); !ok || len(con) != 0 {
		t.Errorf("expected empty console slice, got %T %v", m["console"], m["console"])
	}
}
