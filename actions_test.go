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
		{"wait", `{"type":"wait","wait_ms":1000}`},
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
		{"select_option", `{"type":"select_option","selector":"#lang","values":["en"]}`},
		{"resize", `{"type":"resize","width":800,"height":600}`},
		{"fill_form", `{"type":"fill_form","fields":[{"selector":"#x","value":"y"}]}`},
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

func TestParseAction_WaitForText(t *testing.T) {
	raw := `{"type":"wait_for","text":"Welcome","timeout_ms":5000}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Text != "Welcome" {
		t.Errorf("Text = %q, want %q", a.Text, "Welcome")
	}
	if a.TimeoutMs != 5000 {
		t.Errorf("TimeoutMs = %d, want 5000", a.TimeoutMs)
	}
}

func TestParseAction_WaitForTextGone(t *testing.T) {
	raw := `{"type":"wait_for","text_gone":"Loading...","timeout_ms":3000}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.TextGone != "Loading..." {
		t.Errorf("TextGone = %q, want %q", a.TextGone, "Loading...")
	}
}

func TestParseAction_WaitForCookie(t *testing.T) {
	raw := `{"type":"wait_for","cookie":"_px3","timeout_ms":10000}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Cookie != "_px3" {
		t.Errorf("Cookie = %q, want %q", a.Cookie, "_px3")
	}
	if a.TimeoutMs != 10000 {
		t.Errorf("TimeoutMs = %d, want 10000", a.TimeoutMs)
	}
}

func TestParseAction_SelectOption(t *testing.T) {
	raw := `{"type":"select_option","selector":"#country","values":["Russia","Germany"]}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(a.Values) != 2 || a.Values[0] != "Russia" {
		t.Errorf("Values = %v, want [Russia Germany]", a.Values)
	}
}

func TestParseAction_Resize(t *testing.T) {
	raw := `{"type":"resize","width":1920,"height":1080}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Width != 1920 || a.Height != 1080 {
		t.Errorf("Width=%d Height=%d, want 1920x1080", a.Width, a.Height)
	}
}

func TestParseAction_FillForm(t *testing.T) {
	raw := `{"type":"fill_form","fields":[{"selector":"#name","value":"John"},{"selector":"#agree","value":"true","type":"checkbox"}]}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(a.Fields) != 2 {
		t.Fatalf("Fields len = %d, want 2", len(a.Fields))
	}
	if a.Fields[0].Selector != "#name" || a.Fields[0].Value != "John" {
		t.Errorf("Field[0] = %+v", a.Fields[0])
	}
	if a.Fields[1].Type != "checkbox" {
		t.Errorf("Field[1].Type = %q, want checkbox", a.Fields[1].Type)
	}
}

func TestParseAction_TypeTextSlowly(t *testing.T) {
	raw := `{"type":"type_text","selector":"input","text":"hello","slowly":true,"submit":true}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !a.Slowly {
		t.Error("Slowly = false, want true")
	}
	if !a.Submit {
		t.Error("Submit = false, want true")
	}
}

func TestParseAction_SnapshotDepth(t *testing.T) {
	raw := `{"type":"snapshot","depth":3}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Depth != 3 {
		t.Errorf("Depth = %d, want 3", a.Depth)
	}
}

func TestActionRegistry_WaitForNavigation(t *testing.T) {
	if _, ok := actionRegistry["wait_for_navigation"]; !ok {
		t.Error("wait_for_navigation not registered in actionRegistry")
	}
}

func TestParseAction_ClickModifiers(t *testing.T) {
	raw := `{"type":"click","selector":"a.link","button":"right","double_click":true,"modifiers":["Control","Shift"]}`
	var a Action
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Button != "right" {
		t.Errorf("Button = %q, want %q", a.Button, "right")
	}
	if !a.DoubleClick {
		t.Error("DoubleClick = false, want true")
	}
	if len(a.Modifiers) != 2 || a.Modifiers[0] != "Control" {
		t.Errorf("Modifiers = %v, want [Control Shift]", a.Modifiers)
	}
}
