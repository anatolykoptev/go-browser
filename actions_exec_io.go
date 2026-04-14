package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"time"

	"golang.org/x/image/draw"

	"github.com/anatolykoptev/go-browser/cdputil"
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
		return nil, fmt.Errorf("evaluate: %s: %w", res.ExceptionDetails.Text, ErrJsException)
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
