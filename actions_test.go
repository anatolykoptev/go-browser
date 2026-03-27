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
		{"get_cookies", `{"type":"get_cookies"}`},
		{"destroy_session", `{"type":"destroy_session"}`},
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

func TestParseAction_GetCookies(t *testing.T) {
	raw := `{"type":"get_cookies"}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Type != "get_cookies" {
		t.Errorf("Type = %q, want %q", a.Type, "get_cookies")
	}
}

func TestParseAction_DestroySession(t *testing.T) {
	raw := `{"type":"destroy_session"}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Type != "destroy_session" {
		t.Errorf("Type = %q, want %q", a.Type, "destroy_session")
	}
}

func TestParseAction_EvaluateJSField(t *testing.T) {
	raw := `{"type":"evaluate","js":"return 1"}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.JS != "return 1" {
		t.Errorf("JS = %q, want %q", a.JS, "return 1")
	}
}

func TestParseAction_HandleDialogAccept(t *testing.T) {
	f := false
	raw := `{"type":"handle_dialog","accept":false,"text":"my prompt"}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Accept == nil || *a.Accept != f {
		t.Errorf("Accept = %v, want *false", a.Accept)
	}
	if a.Text != "my prompt" {
		t.Errorf("Text = %q, want %q", a.Text, "my prompt")
	}
}

func TestParseAction_CookieInputSecureHTTPOnly(t *testing.T) {
	raw := `{"type":"set_cookies","cookies":[{"name":"sid","value":"abc","domain":"example.com","secure":true,"http_only":true}]}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(a.Cookies) != 1 {
		t.Fatalf("Cookies len = %d, want 1", len(a.Cookies))
	}
	c := a.Cookies[0]
	if !c.Secure {
		t.Error("Secure = false, want true")
	}
	if !c.HTTPOnly {
		t.Error("HTTPOnly = false, want true")
	}
}
