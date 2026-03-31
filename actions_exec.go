package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// keyMap maps action key names to rod input keys.
//
//nolint:gochecknoglobals // static key mapping
var keyMap = map[string]input.Key{
	"Enter":      input.Enter,
	"Tab":        input.Tab,
	"Escape":     input.Escape,
	"Backspace":  input.Backspace,
	"Delete":     input.Delete,
	"ArrowUp":    input.ArrowUp,
	"ArrowDown":  input.ArrowDown,
	"ArrowLeft":  input.ArrowLeft,
	"ArrowRight": input.ArrowRight,
	"Space":      input.Space,
	"Home":       input.Home,
	"End":        input.End,
	"PageUp":     input.PageUp,
	"PageDown":   input.PageDown,
	"F1":         input.F1,
	"F2":         input.F2,
	"F3":         input.F3,
	"F4":         input.F4,
	"F5":         input.F5,
	"F6":         input.F6,
	"F7":         input.F7,
	"F8":         input.F8,
	"F9":         input.F9,
	"F10":        input.F10,
	"F11":        input.F11,
	"F12":        input.F12,
}

// resolveElement finds an element using CSS, text=, or xpath= selector.
//
//nolint:cyclop // simple prefix dispatch
func resolveElement(ctx context.Context, page *rod.Page, selector string) (*rod.Element, error) {
	p := page.Context(ctx)
	switch {
	case strings.HasPrefix(selector, "text="):
		text := strings.TrimPrefix(selector, "text=")
		return p.ElementR("*", text)
	case strings.HasPrefix(selector, "xpath="):
		xpath := strings.TrimPrefix(selector, "xpath=")
		return p.ElementX(xpath)
	default:
		return p.Element(selector)
	}
}

//nolint:gochecknoglobals // static modifier key mapping
var modifierKeyMap = map[string]input.Key{
	"Alt":     input.AltLeft,
	"Control": input.ControlLeft,
	"Shift":   input.ShiftLeft,
	"Meta":    input.MetaLeft,
}

func holdModifiers(page *rod.Page, modifiers []string) func() {
	var held []input.Key
	for _, m := range modifiers {
		if k, ok := modifierKeyMap[m]; ok {
			_ = page.Keyboard.Press(k)
			held = append(held, k)
		}
	}
	return func() {
		for _, k := range held {
			_ = page.Keyboard.Release(k)
		}
	}
}

func doClick(ctx context.Context, page *rod.Page, a Action) error {
	el, err := resolveElement(ctx, page, a.Selector)
	if err != nil {
		return fmt.Errorf("click: find %q: %w", a.Selector, err)
	}

	release := holdModifiers(page, a.Modifiers)
	defer release()

	btn := proto.InputMouseButtonLeft
	switch a.Button {
	case "right":
		btn = proto.InputMouseButtonRight
	case "middle":
		btn = proto.InputMouseButtonMiddle
	}

	clicks := 1
	if a.DoubleClick {
		clicks = 2
	}

	if err := el.Click(btn, clicks); err != nil {
		return fmt.Errorf("click: %w", err)
	}
	return nil
}

func doTypeText(ctx context.Context, page *rod.Page, selector, text string) error {
	el, err := resolveElement(ctx, page, selector)
	if err != nil {
		return fmt.Errorf("type_text: find %q: %w", selector, err)
	}
	if err := el.SelectAllText(); err != nil {
		return fmt.Errorf("type_text: select all: %w", err)
	}
	if err := el.Input(text); err != nil {
		return fmt.Errorf("type_text: input: %w", err)
	}
	return nil
}

func doWaitFor(ctx context.Context, page *rod.Page, selector string) error {
	if _, err := resolveElement(ctx, page, selector); err != nil {
		return fmt.Errorf("wait_for %q: %w", selector, err)
	}
	return nil
}

// doWaitForText polls until text appears in page body.
func doWaitForText(ctx context.Context, page *rod.Page, text string) error {
	for {
		content, err := proto.RuntimeEvaluate{
			Expression:    "document.body ? document.body.innerText : ''",
			ReturnByValue: true,
		}.Call(page)
		if err == nil {
			if strings.Contains(fmt.Sprintf("%v", content.Result.Value.Val()), text) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_for text %q: %w", text, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// doWaitForTextGone polls until text disappears from page body.
func doWaitForTextGone(ctx context.Context, page *rod.Page, text string) error {
	for {
		content, err := proto.RuntimeEvaluate{
			Expression:    "document.body ? document.body.innerText : ''",
			ReturnByValue: true,
		}.Call(page)
		if err == nil {
			if !strings.Contains(fmt.Sprintf("%v", content.Result.Value.Val()), text) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_for text_gone %q: %w", text, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func doScreenshot(page *rod.Page) (string, error) {
	buf, err := page.Screenshot(true, nil)
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// doEvaluate runs JS as a raw expression via CDP RuntimeEvaluate.
// Unlike rod's page.Eval (which wraps in function(){}.apply()), this accepts
// any JS expression: "document.title", "1+1", "JSON.stringify({a:1})", etc.
func doEvaluate(page *rod.Page, script string) (any, error) {
	res, err := proto.RuntimeEvaluate{
		Expression:    script,
		ReturnByValue: true,
		AwaitPromise:  true,
	}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}
	if res.ExceptionDetails != nil {
		return nil, fmt.Errorf("evaluate: %s", res.ExceptionDetails.Text)
	}
	return res.Result.Value.Val(), nil
}

func doPress(page *rod.Page, key string) error {
	k, ok := keyMap[key]
	if !ok {
		return fmt.Errorf("press: unknown key %q", key)
	}
	if err := page.Keyboard.Press(k); err != nil {
		return fmt.Errorf("press %q: %w", key, err)
	}
	return nil
}

func doSleep(ctx context.Context, waitMs int) error {
	if waitMs <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
		return nil
	}
}

// doNavigate uses rod's Navigate (which resets JS context) + DOMContentLoaded wait.
// rod's Navigate() returns immediately — the hang was in WaitLoad(), not Navigate().
// We replace WaitLoad() with WaitNavigation(DOMContentLoaded) which is SPA-safe.
func doNavigate(ctx context.Context, page *rod.Page, url string) error {
	p := page.Context(ctx)

	// Set up DOMContentLoaded listener BEFORE firing navigation.
	wait := p.WaitNavigation(proto.PageLifecycleEventNameDOMContentLoaded)

	// rod's Navigate: fires proto.PageNavigate + calls unsetJSCtxID().
	// Returns immediately — does NOT wait for load event.
	if err := p.Navigate(url); err != nil {
		return fmt.Errorf("navigate %q: %w", url, err)
	}

	// Block until DOMContentLoaded (bounded by ctx timeout).
	wait()
	return nil
}

func doSetCookies(page *rod.Page, cookies []CookieInput) error {
	for _, c := range cookies {
		req := proto.NetworkSetCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
		}
		if _, err := req.Call(page); err != nil {
			return fmt.Errorf("set_cookies %q: %w", c.Name, err)
		}
	}
	return nil
}

func doGetCookies(page *rod.Page) ([]map[string]any, error) {
	cookies, err := page.Cookies(nil)
	if err != nil {
		return nil, fmt.Errorf("get_cookies: %w", err)
	}
	result := make([]map[string]any, 0, len(cookies))
	for _, c := range cookies {
		result = append(result, map[string]any{
			"name":      c.Name,
			"value":     c.Value,
			"domain":    c.Domain,
			"path":      c.Path,
			"secure":    c.Secure,
			"http_only": c.HTTPOnly,
		})
	}
	return result, nil
}

func doSnapshot(page *rod.Page, _ string) (string, error) {
	res, err := proto.AccessibilityGetFullAXTree{}.Call(page)
	if err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}

	// Build parent→children map.
	type nodeInfo struct {
		role, name string
		children   []string
	}
	nodes := make(map[string]*nodeInfo, len(res.Nodes))
	var rootID string

	for _, node := range res.Nodes {
		if node.Ignored {
			continue
		}
		id := string(node.NodeID)
		role := ""
		if node.Role != nil {
			role = fmt.Sprintf("%v", node.Role.Value.Val())
		}
		name := ""
		if node.Name != nil {
			name = fmt.Sprintf("%v", node.Name.Value.Val())
		}
		childIDs := make([]string, 0, len(node.ChildIDs))
		for _, cid := range node.ChildIDs {
			childIDs = append(childIDs, string(cid))
		}
		nodes[id] = &nodeInfo{role: role, name: name, children: childIDs}
		if node.ParentID == "" && rootID == "" {
			rootID = id
		}
	}

	var sb strings.Builder
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		n, ok := nodes[id]
		if !ok {
			return
		}
		if n.role != "" || n.name != "" {
			indent := strings.Repeat("  ", depth)
			fmt.Fprintf(&sb, "%s[%s] %s\n", indent, n.role, n.name)
		}
		for _, cid := range n.children {
			walk(cid, depth+1)
		}
	}
	if rootID != "" {
		walk(rootID, 0)
	}

	return sb.String(), nil
}

func doHandleDialog(page *rod.Page, accept bool, promptText string) (string, error) {
	wait, handle := page.HandleDialog()
	ev := wait()
	params := &proto.PageHandleJavaScriptDialog{Accept: accept}
	if promptText != "" {
		params.PromptText = promptText
	}
	if err := handle(params); err != nil {
		return "", fmt.Errorf("handle_dialog: %w", err)
	}
	return ev.Message, nil
}

func doHover(ctx context.Context, page *rod.Page, selector string) error {
	el, err := resolveElement(ctx, page, selector)
	if err != nil {
		return fmt.Errorf("hover: find %q: %w", selector, err)
	}
	if err := el.Hover(); err != nil {
		return fmt.Errorf("hover: %w", err)
	}
	return nil
}

func doGoBack(page *rod.Page) error {
	if err := page.NavigateBack(); err != nil {
		return fmt.Errorf("go_back: %w", err)
	}
	return nil
}

func doSelectOption(ctx context.Context, page *rod.Page, selector string, values []string) error {
	el, err := resolveElement(ctx, page, selector)
	if err != nil {
		return fmt.Errorf("select_option: find %q: %w", selector, err)
	}
	if err := el.Select(values, true, rod.SelectorTypeText); err != nil {
		return fmt.Errorf("select_option: %w", err)
	}
	return nil
}

func doScroll(ctx context.Context, page *rod.Page, selector string, dx, dy float64) error {
	if selector != "" {
		el, err := resolveElement(ctx, page, selector)
		if err != nil {
			return fmt.Errorf("scroll: find %q: %w", selector, err)
		}
		if err := el.ScrollIntoView(); err != nil {
			return fmt.Errorf("scroll into view: %w", err)
		}
		return nil
	}
	const scrollSteps = 1
	if err := page.Mouse.Scroll(dx, dy, scrollSteps); err != nil {
		return fmt.Errorf("scroll: %w", err)
	}
	return nil
}
