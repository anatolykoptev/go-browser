package browser

// actions_inspect.go — executor for element_inspect action.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

func init() {
	registerAction("element_inspect", execElementInspect)
}

// ElementInfo describes a DOM element's properties, click mechanism, and listeners.
type ElementInfo struct {
	Tag         string         `json:"tag"`
	ID          string         `json:"id,omitempty"`
	Classes     string         `json:"classes,omitempty"`
	Href        string         `json:"href,omitempty"`
	OnclickAttr string         `json:"onclick_attr,omitempty"`
	Text        string         `json:"text,omitempty"`
	Mechanism   string         `json:"mechanism"`
	Listeners   []ListenerInfo `json:"listeners"`
	ClickScript string         `json:"click_script"`
	FormAction  string         `json:"form_action,omitempty"`
}

// ListenerInfo describes a single event listener attached to an element.
type ListenerInfo struct {
	Type       string `json:"type"`
	UseCapture bool   `json:"use_capture"`
	Once       bool   `json:"once"`
	ScriptID   string `json:"script_id,omitempty"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Handler    string `json:"handler,omitempty"`
}

// domProps holds properties extracted from an element via JS evaluation.
type domProps struct {
	Tag         string `json:"tag"`
	ID          string `json:"id"`
	Classes     string `json:"classes"`
	Href        string `json:"href"`
	OnclickAttr string `json:"onclick_attr"`
	Text        string `json:"text"`
	FormAction  string `json:"form_action"`
	InForm      bool   `json:"in_form"`
}

const maxHandlerLen = 300

const domPropsJS = `(el) => JSON.stringify({
  tag: el.tagName.toLowerCase(),
  id: el.id || "",
  classes: el.className || "",
  href: el.getAttribute("href") || "",
  onclick_attr: el.getAttribute("onclick") || "",
  text: (el.innerText || "").trim().substring(0, 200),
  form_action: el.closest("form") ? (el.closest("form").action || "") : "",
  in_form: !!el.closest("form")
})`

func execElementInspect(dc dispatchContext, a Action) (any, error) {
	if a.Selector == "" {
		return nil, fmt.Errorf("element_inspect: selector required")
	}

	el, err := resolveElement(dc.ctx, dc.page, a.Selector, dc.refMap)
	if err != nil {
		return nil, fmt.Errorf("element_inspect: %w", err)
	}

	// Extract DOM properties via JS.
	res, err := el.Eval(domPropsJS)
	if err != nil {
		return nil, fmt.Errorf("element_inspect: eval props: %w", err)
	}

	var props domProps
	if err := json.Unmarshal([]byte(res.Value.Str()), &props); err != nil {
		return nil, fmt.Errorf("element_inspect: parse props: %w", err)
	}

	// Get event listeners via CDP.
	listeners := getEventListeners(dc, el.Object.ObjectID)

	mechanism := classifyMechanism(props, listeners)
	clickScript := buildClickScript(a.Selector, mechanism, props)

	info := ElementInfo{
		Tag:         props.Tag,
		ID:          props.ID,
		Classes:     props.Classes,
		Href:        props.Href,
		OnclickAttr: props.OnclickAttr,
		Text:        props.Text,
		Mechanism:   mechanism,
		Listeners:   listeners,
		ClickScript: clickScript,
		FormAction:  props.FormAction,
	}

	return info, nil
}

func getEventListeners(dc dispatchContext, oid proto.RuntimeRemoteObjectID) []ListenerInfo {

	result, err := proto.DOMDebuggerGetEventListeners{ObjectID: oid}.Call(dc.page)
	if err != nil {
		return nil
	}

	out := make([]ListenerInfo, 0, len(result.Listeners))
	for _, l := range result.Listeners {
		handler := ""
		if l.Handler != nil {
			handler = l.Handler.Description
			if len(handler) > maxHandlerLen {
				handler = handler[:maxHandlerLen]
			}
		}
		out = append(out, ListenerInfo{
			Type:       l.Type,
			UseCapture: l.UseCapture,
			Once:       l.Once,
			ScriptID:   string(l.ScriptID),
			Line:       l.LineNumber,
			Column:     l.ColumnNumber,
			Handler:    handler,
		})
	}
	return out
}

func classifyMechanism(props domProps, listeners []ListenerInfo) string {
	if props.OnclickAttr != "" {
		return "onclick_attr"
	}

	href := strings.TrimSpace(props.Href)
	if href != "" && href != "#" && !strings.HasPrefix(href, "javascript:void") {
		return "href"
	}

	if props.InForm && (props.Tag == "button" || props.Tag == "input") {
		return "form_submit"
	}

	for _, l := range listeners {
		if l.Type == "click" {
			return "js_closure"
		}
	}

	if href == "#" || strings.HasPrefix(href, "javascript:void") {
		return "js_void_href"
	}

	return "standard"
}

func buildClickScript(selector, mechanism string, props domProps) string {
	sel := escapeJS(selector)

	switch mechanism {
	case "onclick_attr":
		return fmt.Sprintf(`document.querySelector(%s).click()`, sel)
	case "href":
		return fmt.Sprintf(`window.location.href = %s`, escapeJS(props.Href))
	case "form_submit":
		return fmt.Sprintf(`document.querySelector(%s).closest("form").submit()`, sel)
	default:
		return fmt.Sprintf(`document.querySelector(%s).click()`, sel)
	}
}

func escapeJS(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
