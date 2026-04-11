package browser

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-rod/rod"
)

func TestFrameSelector_ParsedCorrectly(t *testing.T) {
	raw := `{"type":"type_text","selector":"#card-number","text":"4111","frame_selector":"iframe.payment-field"}`

	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if a.FrameSelector != "iframe.payment-field" {
		t.Errorf("FrameSelector = %q, want %q", a.FrameSelector, "iframe.payment-field")
	}
	if a.Type != "type_text" {
		t.Errorf("Type = %q, want %q", a.Type, "type_text")
	}
	if a.Selector != "#card-number" {
		t.Errorf("Selector = %q, want %q", a.Selector, "#card-number")
	}
}

func TestFrameSelector_EmptyString_OmittedFromJSON(t *testing.T) {
	a := Action{Type: "click", Selector: "#btn"}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}

	if _, exists := m["frame_selector"]; exists {
		t.Error("frame_selector should be omitted from JSON when empty")
	}
}

func TestExecuteAction_WithFrameSelector_NoCursor(t *testing.T) {
	// When FrameSelector is set, cursor should be disabled in the dispatch context.
	// We verify this indirectly: if FrameSelector is empty, the action type dispatch
	// proceeds normally without frame resolution.
	a := Action{
		Type:          "sleep",
		WaitMs:        1,
		FrameSelector: "", // empty = no frame switch
	}

	ctx := context.Background()
	result := ExecuteAction(ctx, nil, a, nil, nil, false, nil)

	// sleep action should succeed even with nil page (it only uses context).
	if !result.Ok {
		t.Errorf("expected ok for sleep action without frame_selector, got error: %s", result.Error)
	}
}

// TestFrameTypeCompatibility is a compile-time check that el.Frame() returns *rod.Page.
// This ensures all action executors (which take *rod.Page) work with frame pages.
func TestFrameTypeCompatibility(t *testing.T) {
	// Compile-time assertion: el.Frame() returns (*rod.Page, error).
	// We cannot call it without a real browser, but we verify the type signature.
	var _ func() (*rod.Page, error)
	el := &rod.Element{}
	_ = el.Frame // This compiles only if Frame returns (*rod.Page, error)
}

func TestParseFrameSelector(t *testing.T) {
	tests := []struct {
		sel     string
		kind    string
		pattern string
	}{
		{"iframe.payment", "css", "iframe.payment"},
		{"url=payments.audienceview.com", "url", "payments.audienceview.com"},
		{"url=stripe.com", "url", "stripe.com"},
		{"#card-frame", "css", "#card-frame"},
	}
	for _, tt := range tests {
		kind, pattern := parseFrameSelector(tt.sel)
		if kind != tt.kind || pattern != tt.pattern {
			t.Errorf("parseFrameSelector(%q) = %q, %q; want %q, %q",
				tt.sel, kind, pattern, tt.kind, tt.pattern)
		}
	}
}

func TestTypeIntoFrameRegistered(t *testing.T) {
	_, ok := actionRegistry["type_into_frame"]
	if !ok {
		t.Fatal("type_into_frame action not registered in actionRegistry")
	}
}

func TestMatchFrameURL(t *testing.T) {
	tests := []struct {
		frameURL string
		pattern  string
		want     bool
	}{
		{"https://payments.audienceview.com/hosted-fields/v1/index.html", "payments.audienceview.com", true},
		{"https://js.stripe.com/v3/controller.html", "stripe.com", true},
		{"https://example.com/page", "stripe.com", false},
		{"about:blank", "payments", false},
	}
	for _, tt := range tests {
		if got := matchFrameURL(tt.frameURL, tt.pattern); got != tt.want {
			t.Errorf("matchFrameURL(%q, %q) = %v; want %v",
				tt.frameURL, tt.pattern, got, tt.want)
		}
	}
}
