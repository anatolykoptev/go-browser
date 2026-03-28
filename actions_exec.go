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
var keyMap = map[string]input.Key{
	"Enter":      input.Enter,
	"Tab":        input.Tab,
	"Escape":     input.Escape,
	"Backspace":  input.Backspace,
	"ArrowUp":    input.ArrowUp,
	"ArrowDown":  input.ArrowDown,
	"ArrowLeft":  input.ArrowLeft,
	"ArrowRight": input.ArrowRight,
	"Space":      input.Space,
}

func doClick(ctx context.Context, page *rod.Page, selector string) error {
	el, err := page.Context(ctx).Element(selector)
	if err != nil {
		return fmt.Errorf("click: find %q: %w", selector, err)
	}
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("click: %w", err)
	}
	return nil
}

func doTypeText(ctx context.Context, page *rod.Page, selector, text string) error {
	el, err := page.Context(ctx).Element(selector)
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
	if _, err := page.Context(ctx).Element(selector); err != nil {
		return fmt.Errorf("wait_for %q: %w", selector, err)
	}
	return nil
}

func doScreenshot(page *rod.Page) (string, error) {
	buf, err := page.Screenshot(true, nil)
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func doEvaluate(page *rod.Page, script string) (any, error) {
	res, err := page.Eval(script)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}
	return res.Value.Val(), nil
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

func doNavigate(ctx context.Context, page *rod.Page, url string) error {
	if err := page.Context(ctx).Navigate(url); err != nil {
		return fmt.Errorf("navigate %q: %w", url, err)
	}
	if err := page.Context(ctx).WaitLoad(); err != nil {
		return fmt.Errorf("navigate wait_load: %w", err)
	}
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
	el, err := page.Context(ctx).Element(selector)
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

func doScroll(ctx context.Context, page *rod.Page, selector string, dx, dy float64) error {
	if selector != "" {
		el, err := page.Context(ctx).Element(selector)
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
