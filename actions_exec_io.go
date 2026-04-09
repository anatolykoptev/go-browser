package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/anatolykoptev/go-browser/cdputil"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

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

func doResize(page *rod.Page, width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("resize: width and height must be positive")
	}
	return page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:  width,
		Height: height,
	})
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

func doGoBack(page *rod.Page) error {
	if err := page.NavigateBack(); err != nil {
		return fmt.Errorf("go_back: %w", err)
	}
	return nil
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
