package browser

import (
	"encoding/json"
	"testing"
)

func TestParseAction_Click(t *testing.T) {
	raw := `{"type":"click","selector":"#submit"}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Type != "click" {
		t.Errorf("Type = %q, want %q", a.Type, "click")
	}
	if a.Selector != "#submit" {
		t.Errorf("Selector = %q, want %q", a.Selector, "#submit")
	}
}

func TestParseAction_TypeText(t *testing.T) {
	raw := `{"type":"type_text","selector":"input[name=q]","text":"hello world"}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Type != "type_text" {
		t.Errorf("Type = %q, want %q", a.Type, "type_text")
	}
	if a.Text != "hello world" {
		t.Errorf("Text = %q, want %q", a.Text, "hello world")
	}
}

func TestParseAction_AllTypes(t *testing.T) {
	types := []struct {
		name string
		json string
	}{
		{"click", `{"type":"click","selector":"a"}`},
		{"type_text", `{"type":"type_text","selector":"input","text":"x"}`},
		{"wait_for", `{"type":"wait_for","selector":".ready"}`},
		{"screenshot", `{"type":"screenshot"}`},
		{"evaluate", `{"type":"evaluate","script":"return 1"}`},
		{"press", `{"type":"press","key":"Enter"}`},
		{"sleep", `{"type":"sleep","wait_ms":500}`},
		{"navigate", `{"type":"navigate","url":"https://example.com"}`},
		{"set_cookies", `{"type":"set_cookies","cookies":[{"name":"sid","value":"abc","domain":"example.com"}]}`},
		{"snapshot", `{"type":"snapshot","format":"text"}`},
		{"handle_dialog", `{"type":"handle_dialog"}`},
		{"hover", `{"type":"hover","selector":"button"}`},
		{"go_back", `{"type":"go_back"}`},
		{"get_logs", `{"type":"get_logs"}`},
		{"scroll", `{"type":"scroll","selector":".item","delta_x":0,"delta_y":300}`},
	}

	for _, tc := range types {
		t.Run(tc.name, func(t *testing.T) {
			var a Action
			if err := json.Unmarshal([]byte(tc.json), &a); err != nil {
				t.Fatalf("unmarshal %q: %v", tc.name, err)
			}
			if a.Type != tc.name {
				t.Errorf("Type = %q, want %q", a.Type, tc.name)
			}
		})
	}
}
