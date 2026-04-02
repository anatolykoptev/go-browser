package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/anatolykoptev/go-browser/cdputil"
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

func doTypeText(ctx context.Context, page *rod.Page, selector, text string, slowly, submit bool) error {
	if slowly {
		// CDP char-by-char path — for bot-detection protected pages (LinkedIn, etc.).
		// Uses JS focus + CDP dispatchKeyEvent which triggers React onChange
		// and bypasses PX/bot-detection event interception.
		return doTypeTextCDP(ctx, page, selector, text, submit)
	}

	// Default fast path — rod's Input() via Runtime.callFunctionOn.
	// Works for most sites. Falls back to CDP path on timeout.
	el, err := resolveElement(ctx, page, selector)
	if err != nil {
		return fmt.Errorf("type_text: find %q: %w", selector, err)
	}
	_ = el.SelectAllText()
	if err := el.Input(text); err != nil {
		return fmt.Errorf("type_text: input: %w", err)
	}
	if submit {
		if err := page.Keyboard.Press(input.Enter); err != nil {
			return fmt.Errorf("type_text: submit: %w", err)
		}
	}
	return nil
}

// doTypeTextCDP types text using pure CDP events — reliable on PX-protected pages.
// Focus via CDP DOM.focus (no Runtime.callFunctionOn), clear via Ctrl+A+Delete, type via dispatchKeyEvent.
func doTypeTextCDP(ctx context.Context, page *rod.Page, selector, text string, submit bool) error {
	nodeID, err := cdputil.QuerySelector(page, selector)
	if err != nil {
		return fmt.Errorf("type_text: %w", err)
	}
	if err := cdputil.FocusNode(page, nodeID); err != nil {
		return fmt.Errorf("type_text: focus: %w", err)
	}

	// Clear via Ctrl+A then Delete.
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: "a", Code: "KeyA",
		WindowsVirtualKeyCode: 65, Modifiers: 2,
	}).Call(page)
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeKeyUp, Key: "a", Code: "KeyA",
	}).Call(page)
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: "Delete", Code: "Delete",
		WindowsVirtualKeyCode: 46,
	}).Call(page)
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeKeyUp, Key: "Delete", Code: "Delete",
	}).Call(page)

	for _, ch := range text {
		char := string(ch)
		code := charToCode(ch)
		vk := charToVK(ch)

		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: char, Code: code,
			WindowsVirtualKeyCode: vk,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeChar, Text: char, UnmodifiedText: char,
			WindowsVirtualKeyCode: vk,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeKeyUp, Key: char, Code: code,
			WindowsVirtualKeyCode: vk,
		}).Call(page)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}

	if submit {
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: "Enter", Code: "Enter",
			WindowsVirtualKeyCode: 13,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeKeyUp, Key: "Enter", Code: "Enter",
			WindowsVirtualKeyCode: 13,
		}).Call(page)
	}
	return nil
}

func doFillForm(ctx context.Context, page *rod.Page, fields []FormField) error {
	for _, f := range fields {
		el, err := resolveElement(ctx, page, f.Selector)
		if err != nil {
			return fmt.Errorf("fill_form: find %q: %w", f.Selector, err)
		}
		switch f.Type {
		case "checkbox":
			checked, _ := el.Property("checked")
			want := f.Value == "true"
			if checked.Bool() != want {
				if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
					return fmt.Errorf("fill_form: checkbox %q: %w", f.Selector, err)
				}
			}
		case "combobox":
			if err := el.Select([]string{f.Value}, true, rod.SelectorTypeText); err != nil {
				return fmt.Errorf("fill_form: select %q: %w", f.Selector, err)
			}
		default: // textbox
			_ = el.SelectAllText()
			if err := el.Input(f.Value); err != nil {
				return fmt.Errorf("fill_form: input %q: %w", f.Selector, err)
			}
		}
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

// doWaitForCookie polls until a cookie with the given name appears.
// Used for PerimeterX (_px3), DataDome (datadome), CF (__cf_bm) challenge cookies.
func doWaitForCookie(ctx context.Context, page *rod.Page, name string) error {
	for {
		cookies, err := page.Cookies(nil)
		if err == nil {
			for _, c := range cookies {
				if c.Name == name {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_for cookie %q: %w", name, ctx.Err())
		case <-time.After(500 * time.Millisecond):
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

// doNavigate uses rod's Navigate + WaitRequestIdle for SPA-safe navigation.
// WaitRequestIdle excludes WebSocket/SSE by default — won't hang on Twitter.
// Navigation wait is capped at 7s — sites with continuous HTTP polling (Yandex, ad-heavy)
// never reach 500ms of silence, so we proceed after the cap.
func doNavigate(ctx context.Context, page *rod.Page, url string) error {
	// Cap navigation wait: don't block more than 7s waiting for idle.
	navCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()

	p := page.Context(navCtx)

	// Set up network idle listener BEFORE firing navigation.
	// Default excludeTypes: WebSocket, EventSource, Media, Image, Font.
	waitIdle := p.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)

	// rod's Navigate: fires proto.PageNavigate + calls unsetJSCtxID().
	// Returns immediately — does NOT wait for load event.
	if err := page.Context(ctx).Navigate(url); err != nil {
		return fmt.Errorf("navigate %q: %w", url, err)
	}

	// Block until 500ms of HTTP silence or 7s cap (whichever comes first).
	waitIdle()
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

func doSnapshot(page *rod.Page, maxDepth int) (string, error) {
	// Collect AX trees from main frame + all child frames.
	allNodes := collectAXNodes(page, proto.PageFrameID(""))

	// Also collect from child iframes.
	frames, err := proto.PageGetFrameTree{}.Call(page)
	if err == nil {
		walkFrameTree(page, frames.FrameTree, &allNodes)
	}

	return renderAXTree(allNodes, maxDepth), nil
}

// walkFrameTree recursively visits all child frames and appends their AX nodes.
func walkFrameTree(page *rod.Page, tree *proto.PageFrameTree, allNodes *[]*proto.AccessibilityAXNode) {
	for _, child := range tree.ChildFrames {
		childNodes := collectAXNodes(page, proto.PageFrameID(child.Frame.ID))
		*allNodes = append(*allNodes, childNodes...)
		walkFrameTree(page, child, allNodes)
	}
}

// collectAXNodes fetches the accessibility tree for a single frame.
func collectAXNodes(page *rod.Page, frameID proto.PageFrameID) []*proto.AccessibilityAXNode {
	req := proto.AccessibilityGetFullAXTree{}
	if frameID != "" {
		req.FrameID = frameID
	}
	res, err := req.Call(page)
	if err != nil {
		return nil
	}
	return res.Nodes
}

// renderAXTree builds a text representation of the accessibility tree.
func renderAXTree(nodes []*proto.AccessibilityAXNode, maxDepth int) string {
	type nodeInfo struct {
		role, name string
		children   []string
	}
	index := make(map[string]*nodeInfo, len(nodes))
	var rootID string

	for _, node := range nodes {
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
		index[id] = &nodeInfo{role: role, name: name, children: childIDs}
		if node.ParentID == "" && rootID == "" {
			rootID = id
		}
	}

	var sb strings.Builder
	var walk func(id string, level int)
	walk = func(id string, level int) {
		if maxDepth > 0 && level >= maxDepth {
			return
		}
		n, ok := index[id]
		if !ok {
			return
		}
		if n.role != "" || n.name != "" {
			indent := strings.Repeat("  ", level)
			fmt.Fprintf(&sb, "%s[%s] %s\n", indent, n.role, n.name)
		}
		for _, cid := range n.children {
			walk(cid, level+1)
		}
	}
	if rootID != "" {
		walk(rootID, 0)
	}

	return sb.String()
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

func doResize(page *rod.Page, width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("resize: width and height must be positive")
	}
	return page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  width,
		Height: height,
	})
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

// --- Stealth action variants (use cdputil, no Runtime.callFunctionOn) ---

func doClickStealth(ctx context.Context, page *rod.Page, a Action) error {
	nodeID, err := cdputil.QuerySelector(page, a.Selector)
	if err != nil {
		return fmt.Errorf("click: %w", err)
	}
	_ = cdputil.ScrollIntoView(page, nodeID) // best-effort

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

	return cdputil.ClickNode(page, nodeID, btn, clicks)
}

func doFillFormStealth(ctx context.Context, page *rod.Page, fields []FormField) error {
	for _, f := range fields {
		switch f.Type {
		case "checkbox":
			nodeID, err := cdputil.QuerySelector(page, f.Selector)
			if err != nil {
				return fmt.Errorf("fill_form: find %q: %w", f.Selector, err)
			}
			if err := cdputil.ClickNode(page, nodeID, proto.InputMouseButtonLeft, 1); err != nil {
				return fmt.Errorf("fill_form: checkbox %q: %w", f.Selector, err)
			}
		default:
			if err := doTypeTextCDP(ctx, page, f.Selector, f.Value, false); err != nil {
				return fmt.Errorf("fill_form: %w", err)
			}
		}
	}
	return nil
}

func doHoverStealth(ctx context.Context, page *rod.Page, selector string) error {
	nodeID, err := cdputil.QuerySelector(page, selector)
	if err != nil {
		return fmt.Errorf("hover: %w", err)
	}
	x, y, err := cdputil.NodeCenter(page, nodeID)
	if err != nil {
		return fmt.Errorf("hover: %w", err)
	}
	return (proto.InputDispatchMouseEvent{
		Type: proto.InputDispatchMouseEventTypeMouseMoved,
		X:    x,
		Y:    y,
	}).Call(page)
}

func doWaitForStealth(ctx context.Context, page *rod.Page, selector string) error {
	for {
		_, err := cdputil.QuerySelector(page, selector)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_for %q: %w", selector, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}
