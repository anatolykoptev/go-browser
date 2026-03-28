package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
)

// Action describes a single Chrome interaction step.
type Action struct {
	Type      string        `json:"type"`
	Selector  string        `json:"selector,omitempty"`
	Text      string        `json:"text,omitempty"`
	Script    string        `json:"script,omitempty"`
	JS        string        `json:"js,omitempty"`
	Key       string        `json:"key,omitempty"`
	URL       string        `json:"url,omitempty"`
	Humanize  bool          `json:"humanize,omitempty"`
	WaitMs    int           `json:"wait_ms,omitempty"`
	TimeoutMs int           `json:"timeout_ms,omitempty"`
	Format    string        `json:"format,omitempty"`
	Cookies   []CookieInput `json:"cookies,omitempty"`
	DeltaX    float64       `json:"delta_x,omitempty"`
	DeltaY    float64       `json:"delta_y,omitempty"`
	Accept    *bool         `json:"accept,omitempty"`
}

// CookieInput holds cookie data for the set_cookies action.
type CookieInput struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
}

// ActionResult is the outcome of a single executed action.
type ActionResult struct {
	Action string `json:"action"`
	Ok     bool   `json:"ok"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ExecuteAction dispatches to the correct do* function based on a.Type.
// When cursor is non-nil and a.Humanize is true, humanized variants are used
// for click, type_text, and hover actions.
func ExecuteAction( //nolint:cyclop // dispatch switch — complexity inherent
	ctx context.Context, page *rod.Page, a Action, cursor *humanize.Cursor, logs *LogCollector,
) ActionResult {
	var (
		data any
		err  error
	)

	switch a.Type {
	case "click":
		if a.Humanize && cursor != nil {
			err = doClickHumanized(ctx, page, a.Selector, cursor)
		} else {
			err = doClick(ctx, page, a.Selector)
		}
	case "type_text":
		if a.Humanize && cursor != nil {
			err = doTypeTextHumanized(ctx, page, a.Selector, a.Text, cursor)
		} else {
			err = doTypeText(ctx, page, a.Selector, a.Text)
		}
	case "wait_for":
		if a.TimeoutMs > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(a.TimeoutMs)*time.Millisecond)
			err = doWaitFor(ctx, page, a.Selector)
			cancel()
		} else {
			err = doWaitFor(ctx, page, a.Selector)
		}
	case "screenshot":
		data, err = doScreenshot(page)
	case "evaluate":
		script := a.Script
		if script == "" {
			script = a.JS
		}
		data, err = doEvaluate(page, script)
	case "press":
		err = doPress(page, a.Key)
	case "sleep":
		err = doSleep(ctx, a.WaitMs)
	case "navigate":
		err = doNavigate(ctx, page, a.URL)
	case "set_cookies":
		err = doSetCookies(page, a.Cookies)
	case "snapshot":
		data, err = doSnapshot(page, a.Format)
	case "handle_dialog":
		accept := true
		if a.Accept != nil {
			accept = *a.Accept
		}
		data, err = doHandleDialog(page, accept, a.Text)
	case "get_cookies":
		data, err = doGetCookies(page)
	case "destroy_session":
		// No-op in action execution — session lifecycle managed by handler
	case "hover":
		if a.Humanize && cursor != nil {
			err = doHoverHumanized(ctx, page, a.Selector, cursor)
		} else {
			err = doHover(ctx, page, a.Selector)
		}
	case "go_back":
		err = doGoBack(page)
	case "get_logs":
		if logs != nil {
			net, con := logs.Collect()
			data = map[string]any{"network": net, "console": con}
		} else {
			data = map[string]any{"network": []NetworkEntry{}, "console": []ConsoleEntry{}}
		}
	case "warmup":
		waitMs := a.WaitMs
		if waitMs <= 0 {
			waitMs = 3000
		}
		var count int
		count, err = doWarmup(ctx, page, waitMs, cursor)
		data = count
	case "scroll":
		err = doScroll(ctx, page, a.Selector, a.DeltaX, a.DeltaY)
	default:
		err = fmt.Errorf("unknown action type: %q", a.Type)
	}

	if err != nil {
		return ActionResult{Action: a.Type, Ok: false, Error: err.Error()}
	}
	return ActionResult{Action: a.Type, Ok: true, Data: data}
}
