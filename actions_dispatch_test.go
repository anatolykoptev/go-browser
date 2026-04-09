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
	result := ExecuteAction(context.Background(), nil, Action{Type: "nonexistent_action"}, nil, nil, false)
	if result.Ok {
		t.Error("expected Ok=false for unknown action type")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error for unknown action type")
	}
}

// TestExecuteAction_DestroySessionNoOp verifies destroy_session returns Ok=true with no data.
func TestExecuteAction_DestroySessionNoOp(t *testing.T) {
	result := ExecuteAction(context.Background(), nil, Action{Type: "destroy_session"}, nil, nil, false)
	if !result.Ok {
		t.Errorf("destroy_session: expected Ok=true, got error: %s", result.Error)
	}
	if result.Data != nil {
		t.Errorf("destroy_session: expected nil Data, got %v", result.Data)
	}
}

// TestActionRegistry_Count verifies the registry has the expected number of entries.
func TestActionRegistry_Count(t *testing.T) {
	const wantCount = 23
	if got := len(actionRegistry); got != wantCount {
		t.Errorf("actionRegistry has %d entries, want %d", got, wantCount)
	}
}
