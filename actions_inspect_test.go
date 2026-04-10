package browser

import "testing"

func TestElementInspectAction_Registered(t *testing.T) {
	if _, ok := actionRegistry["element_inspect"]; !ok {
		t.Error("element_inspect not registered in actionRegistry")
	}
}

func TestClassifyMechanism_OnclickAttr(t *testing.T) {
	props := domProps{Tag: "button", OnclickAttr: "doSomething()"}
	got := classifyMechanism(props, nil)
	if got != "onclick_attr" {
		t.Errorf("mechanism = %q, want onclick_attr", got)
	}
}

func TestClassifyMechanism_Href(t *testing.T) {
	props := domProps{Tag: "a", Href: "https://example.com"}
	got := classifyMechanism(props, nil)
	if got != "href" {
		t.Errorf("mechanism = %q, want href", got)
	}
}

func TestClassifyMechanism_FormSubmit(t *testing.T) {
	props := domProps{Tag: "button", InForm: true}
	got := classifyMechanism(props, nil)
	if got != "form_submit" {
		t.Errorf("mechanism = %q, want form_submit", got)
	}
}

func TestClassifyMechanism_InputFormSubmit(t *testing.T) {
	props := domProps{Tag: "input", InForm: true}
	got := classifyMechanism(props, nil)
	if got != "form_submit" {
		t.Errorf("mechanism = %q, want form_submit", got)
	}
}

func TestClassifyMechanism_JSClosure(t *testing.T) {
	props := domProps{Tag: "div"}
	listeners := []ListenerInfo{{Type: "click"}}
	got := classifyMechanism(props, listeners)
	if got != "js_closure" {
		t.Errorf("mechanism = %q, want js_closure", got)
	}
}

func TestClassifyMechanism_JSVoidHref(t *testing.T) {
	props := domProps{Tag: "a", Href: "javascript:void(0)"}
	got := classifyMechanism(props, nil)
	if got != "js_void_href" {
		t.Errorf("mechanism = %q, want js_void_href", got)
	}
}

func TestClassifyMechanism_HashHref(t *testing.T) {
	props := domProps{Tag: "a", Href: "#"}
	got := classifyMechanism(props, nil)
	if got != "js_void_href" {
		t.Errorf("mechanism = %q, want js_void_href", got)
	}
}

func TestClassifyMechanism_Standard(t *testing.T) {
	props := domProps{Tag: "div"}
	got := classifyMechanism(props, nil)
	if got != "standard" {
		t.Errorf("mechanism = %q, want standard", got)
	}
}

func TestClassifyMechanism_Priority_OnclickOverHref(t *testing.T) {
	props := domProps{Tag: "a", Href: "https://example.com", OnclickAttr: "go()"}
	got := classifyMechanism(props, nil)
	if got != "onclick_attr" {
		t.Errorf("mechanism = %q, want onclick_attr (onclick beats href)", got)
	}
}

func TestClassifyMechanism_Priority_HrefOverForm(t *testing.T) {
	props := domProps{Tag: "button", Href: "https://example.com", InForm: true}
	got := classifyMechanism(props, nil)
	if got != "href" {
		t.Errorf("mechanism = %q, want href (href beats form_submit)", got)
	}
}

func TestBuildClickScript_Onclick(t *testing.T) {
	script := buildClickScript("#btn", "onclick_attr", domProps{})
	want := `document.querySelector("#btn").click()`
	if script != want {
		t.Errorf("script = %q, want %q", script, want)
	}
}

func TestBuildClickScript_Href(t *testing.T) {
	script := buildClickScript("a.link", "href", domProps{Href: "https://x.com"})
	want := `window.location.href = "https://x.com"`
	if script != want {
		t.Errorf("script = %q, want %q", script, want)
	}
}

func TestBuildClickScript_FormSubmit(t *testing.T) {
	script := buildClickScript("#submit", "form_submit", domProps{})
	want := `document.querySelector("#submit").closest("form").submit()`
	if script != want {
		t.Errorf("script = %q, want %q", script, want)
	}
}

func TestBuildClickScript_Standard(t *testing.T) {
	script := buildClickScript("div.box", "standard", domProps{})
	want := `document.querySelector("div.box").click()`
	if script != want {
		t.Errorf("script = %q, want %q", script, want)
	}
}

func TestEscapeJS(t *testing.T) {
	got := escapeJS(`he said "hello"`)
	want := `"he said \"hello\""`
	if got != want {
		t.Errorf("escapeJS = %q, want %q", got, want)
	}
}
