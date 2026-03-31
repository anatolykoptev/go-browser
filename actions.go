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
	Type      string        `json:"type" jsonschema:"enum=click,enum=type_text,enum=wait_for,enum=screenshot,enum=evaluate,enum=eval_on_new_document,enum=press,enum=sleep,enum=navigate,enum=set_cookies,enum=snapshot,enum=handle_dialog,enum=get_cookies,enum=destroy_session,enum=hover,enum=go_back,enum=get_logs,enum=warmup,enum=scroll,description=Action type. Use 'snapshot' to get page accessibility tree as text (best for AI). Use 'screenshot' only when visual image is needed."`
	Selector  string        `json:"selector,omitempty" jsonschema:"description=CSS selector for click/type_text/wait_for/hover/scroll"`
	Text      string        `json:"text,omitempty" jsonschema:"description=Text to type (type_text) or prompt response (handle_dialog)"`
	Script    string        `json:"script,omitempty" jsonschema:"description=JavaScript code for evaluate/eval_on_new_document"`
	JS        string        `json:"js,omitempty" jsonschema:"description=Alias for script"`
	Key       string        `json:"key,omitempty" jsonschema:"description=Key name for press action (Enter/Tab/Escape/etc)"`
	URL       string        `json:"url,omitempty" jsonschema:"description=URL for navigate action"`
	Humanize  bool          `json:"humanize,omitempty" jsonschema:"description=Use human-like mouse movement for click/type_text/hover"`
	WaitMs    int           `json:"wait_ms,omitempty" jsonschema:"description=Milliseconds to wait (sleep) or warmup duration"`
	TimeoutMs int           `json:"timeout_ms,omitempty" jsonschema:"description=Timeout in ms for wait_for action"`
	Format    string        `json:"format,omitempty"`
	Cookies   []CookieInput `json:"cookies,omitempty" jsonschema:"description=Cookies for set_cookies action"`
	DeltaX    float64       `json:"delta_x,omitempty" jsonschema:"description=Horizontal scroll delta for scroll action"`
	DeltaY    float64       `json:"delta_y,omitempty" jsonschema:"description=Vertical scroll delta for scroll action"`
	Accept    *bool         `json:"accept,omitempty" jsonschema:"description=Accept or dismiss dialog (handle_dialog)"`
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
	case "eval_on_new_document":
		script := a.Script
		if script == "" {
			script = a.JS
		}
		_, err = page.EvalOnNewDocument(script)
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
