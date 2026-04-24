package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"time"

	"golang.org/x/image/draw"

	"github.com/anatolykoptev/go-browser/cdputil"
	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// screenshotMaxWidth is the max width sent to LLMs.
// Claude processes 16:9 at 1456x819 internally — anything larger is downscaled and wastes tokens.
// browser-use uses 1400x850, Anthropic computer-use uses 1280x800.
const screenshotMaxWidth = 1280

// screenshotJPEGQuality controls CDP JPEG quality (1-100).
// 80 gives ~60-70% size reduction vs PNG while keeping text readable.
const screenshotJPEGQuality = 80

// resizeJPEG decodes a JPEG, scales it down to maxWidth, and re-encodes.
// Returns original data unchanged if already within maxWidth.
func resizeJPEG(data []byte, maxWidth int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	if bounds.Dx() <= maxWidth {
		return data, nil
	}
	scale := float64(maxWidth) / float64(bounds.Dx())
	newH := int(float64(bounds.Dy()) * scale)
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, newH))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: screenshotJPEGQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func doScreenshot(page *rod.Page, fullPage bool) (string, error) {
	// CDP JPEG directly — no PNG→decode→JPEG roundtrip. ~60-70% smaller than PNG.
	q := screenshotJPEGQuality
	req := proto.PageCaptureScreenshot{
		Format:                proto.PageCaptureScreenshotFormatJpeg,
		Quality:               &q,
		CaptureBeyondViewport: fullPage,
	}
	res, err := req.Call(page)
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}

	data, err := resizeJPEG(res.Data, screenshotMaxWidth)
	if err != nil {
		data = res.Data
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// doEvaluate runs JS as a raw expression via CDP RuntimeEvaluate.
// Unlike rod's page.Eval (which wraps in function(){}.apply()), this accepts
// any JS expression: "document.title", "1+1", "JSON.stringify({a:1})", etc.
//
// The raw expression is wrapped in an async IIFE that JSON.stringify's the
// result in the renderer. CDP then returns a plain string we unmarshal in Go.
// Why: ReturnByValue + DOM-adjacent values (Element refs, live NodeList, window)
// trip CDP's "Object reference chain is too long" (-32000), which the naive
// path surfaces as ErrJsException even though JS never threw. Stringifying
// inside the renderer sidesteps that — JSON.stringify naturally serializes
// primitives / plain objects and yields null for DOM nodes / functions.
//
// Caveats:
//   - Scripts that rely on `Runtime.evaluate`'s last-completion-value from a
//     top-level statement (e.g. `var x=1; x+1` without `return`) are no longer
//     supported — callers must write an expression or an IIFE returning a value.
//   - Unserializable results (functions, circular refs) now return nil instead
//     of the previous garbled value; callers relying on the old garbage should
//     stringify themselves.
func doEvaluate(page *rod.Page, script string) (any, error) {
	// Renderer-side wrap: evaluate the caller's script in an inner arrow so bare
	// expressions (`document.title`) and IIFEs (`(()=>{...})()`) both work via
	// `return (SCRIPT)`. The outer async IIFE awaits Promises and stringifies
	// the final value; catch covers both user-thrown errors and JSON.stringify
	// failures (circular refs, BigInt), distinguishing them with __evalError.
	wrapped := `(async function(){
  try {
    const __v = await ((function(){ return (` + script + `); })());
    return JSON.stringify(__v === undefined ? null : __v);
  } catch (__e) {
    return JSON.stringify({__evalError: true, __message: (__e && __e.stack) ? String(__e.stack) : String(__e)});
  }
})()`

	res, err := proto.RuntimeEvaluate{
		Expression:    wrapped,
		ReturnByValue: true,
		AwaitPromise:  true,
	}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}
	if res.ExceptionDetails != nil {
		// Our wrapper catches user exceptions, so any ExceptionDetails here
		// is either a syntax error in the wrapped script or a CDP-side failure.
		return nil, fmt.Errorf("evaluate: %s: %w", res.ExceptionDetails.Text, ErrJsException)
	}

	raw, ok := res.Result.Value.Val().(string)
	if !ok {
		// Wrapper always returns a string; if CDP gave us something else the
		// page may have overridden JSON.stringify. Surface the raw value.
		return res.Result.Value.Val(), nil
	}
	if raw == "" {
		return nil, nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		// Wrapper should always produce valid JSON; if not, return raw string.
		return raw, nil
	}

	// User script threw inside the inner IIFE — surface as js_exception with
	// the original JS error message.
	if m, ok := decoded.(map[string]any); ok {
		if flag, _ := m["__evalError"].(bool); flag {
			msg, _ := m["__message"].(string)
			if msg == "" {
				msg = "unknown"
			}
			return nil, fmt.Errorf("evaluate: %s: %w", msg, ErrJsException)
		}
	}
	return decoded, nil
}

// modifierBitmask converts a list of modifier names into the CDP
// Input.dispatchKeyEvent modifiers bitmask (Alt=1, Ctrl=2, Meta=4, Shift=8).
// Unknown names are ignored.
func modifierBitmask(modifiers []string) int {
	var m int
	for _, mod := range modifiers {
		switch mod {
		case "Alt":
			m |= 1
		case "Control":
			m |= 2
		case "Meta":
			m |= 4
		case "Shift":
			m |= 8
		}
	}
	return m
}

// doPress sends a keystroke to the focused element.
// Named keys (Enter/Tab/F1-F12/arrows/etc) go through rod's keyboard with
// optional modifier hold. Single printable ASCII characters (letters/digits/
// punctuation) are sent via raw CDP Input.dispatchKeyEvent so modifier combos
// like Ctrl+A, Cmd+V, Shift+A work — CDP supports arbitrary keys even though
// rod's named-key registry doesn't.
func doPress(page *rod.Page, key string, modifiers []string) error {
	if key == "" {
		return fmt.Errorf("press: empty key")
	}
	if k, ok := keyMap[key]; ok {
		release := holdModifiers(page, modifiers)
		defer release()
		if err := page.Keyboard.Press(k); err != nil {
			return fmt.Errorf("press %q: %w", key, err)
		}
		return nil
	}
	// Single-character path: CDP dispatchKeyEvent with modifier bitmask.
	runes := []rune(key)
	if len(runes) != 1 {
		return fmt.Errorf("press: unknown key %q", key)
	}
	ch := runes[0]
	if ch > 0x7E || ch < 0x20 {
		return fmt.Errorf("press: unsupported key %q", key)
	}
	ci := humanize.LookupChar(ch)
	if ci.VK == 0 {
		return fmt.Errorf("press: unsupported key %q", key)
	}
	mods := modifierBitmask(modifiers)
	// If the character itself is shifted (e.g. "A", "!") implicitly add Shift.
	if ci.Shift {
		mods |= 8
	}
	if err := (proto.InputDispatchKeyEvent{
		Type:                  proto.InputDispatchKeyEventTypeRawKeyDown,
		Key:                   key,
		Code:                  ci.Code,
		WindowsVirtualKeyCode: ci.VK,
		Modifiers:             mods,
	}).Call(page); err != nil {
		return fmt.Errorf("press %q: keydown: %w", key, err)
	}
	// Only emit the Char event when no non-shift modifier is held — otherwise
	// Ctrl+A would insert the literal "a" on some pages.
	if mods&^8 == 0 {
		if err := (proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeChar,
			Text:                  key,
			UnmodifiedText:        key,
			WindowsVirtualKeyCode: ci.VK,
			Modifiers:             mods,
		}).Call(page); err != nil {
			return fmt.Errorf("press %q: char: %w", key, err)
		}
	}
	if err := (proto.InputDispatchKeyEvent{
		Type:                  proto.InputDispatchKeyEventTypeKeyUp,
		Key:                   key,
		Code:                  ci.Code,
		WindowsVirtualKeyCode: ci.VK,
		Modifiers:             mods,
	}).Call(page); err != nil {
		return fmt.Errorf("press %q: keyup: %w", key, err)
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

func doScroll(ctx context.Context, page *rod.Page, selector string, dx, dy float64, refMap *RefMap) error {
	if selector != "" {
		el, err := resolveElement(ctx, page, selector, refMap)
		if err != nil {
			return fmt.Errorf("scroll: find %q: %w", selector, err)
		}
		// If delta is provided, scroll inside the container element.
		if dx != 0 || dy != 0 {
			_, err := el.Eval(`function(dx, dy) { this.scrollBy(dx, dy) }`, dx, dy)
			if err != nil {
				return fmt.Errorf("scroll container: %w", err)
			}
			return nil
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

func doSelectOption(ctx context.Context, page *rod.Page, selector string, values []string, refMap *RefMap) error {
	el, err := resolveElement(ctx, page, selector, refMap)
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

func doHover(ctx context.Context, page *rod.Page, selector string, refMap *RefMap) error {
	el, err := resolveElement(ctx, page, selector, refMap)
	if err != nil {
		return fmt.Errorf("hover: find %q: %w", selector, err)
	}
	if err := el.Hover(); err != nil {
		return fmt.Errorf("hover: %w", err)
	}
	return nil
}

func doHoverStealth(ctx context.Context, page *rod.Page, selector string, refMap *RefMap) error {
	nodeID, err := resolveRefNodeID(page, selector, refMap)
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
