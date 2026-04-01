package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/anatolykoptev/go-browser/cdputil"
	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
)

// Action describes a single Chrome interaction step.
type Action struct {
	Type        string        `json:"type" jsonschema:"Action type: click, type_text, wait_for (CSS selector, text, text_gone, or wait_ms for time-based wait), snapshot (accessibility tree — best for AI), screenshot (PNG image — only when visual needed), evaluate (any JS expression), eval_on_new_document, press (supports F1-F12), sleep/wait, navigate, set_cookies, handle_dialog, get_cookies, destroy_session, hover, go_back, get_logs, warmup, scroll, select_option (select dropdown values by text). Selectors support CSS, text=, xpath= prefixes. Prefer snapshot over screenshot."`
	Selector    string        `json:"selector,omitempty" jsonschema:"CSS selector for click/type_text/wait_for/hover/scroll"`
	Text        string        `json:"text,omitempty" jsonschema:"Text to type (type_text) or prompt response (handle_dialog)"`
	Script      string        `json:"script,omitempty" jsonschema:"JavaScript code for evaluate/eval_on_new_document"`
	JS          string        `json:"js,omitempty" jsonschema:"Alias for script"`
	Key         string        `json:"key,omitempty" jsonschema:"Key name for press action (Enter/Tab/Escape/etc)"`
	URL         string        `json:"url,omitempty" jsonschema:"URL for navigate action"`
	Humanize    bool          `json:"humanize,omitempty" jsonschema:"Use human-like mouse movement for click/type_text/hover"`
	WaitMs      int           `json:"wait_ms,omitempty" jsonschema:"Milliseconds to wait (sleep) or warmup duration"`
	TimeoutMs   int           `json:"timeout_ms,omitempty" jsonschema:"Timeout in ms for wait_for action"`
	Format      string        `json:"format,omitempty"`
	Cookies     []CookieInput `json:"cookies,omitempty" jsonschema:"Cookies for set_cookies action"`
	DeltaX      float64       `json:"delta_x,omitempty" jsonschema:"Horizontal scroll delta for scroll action"`
	DeltaY      float64       `json:"delta_y,omitempty" jsonschema:"Vertical scroll delta for scroll action"`
	Accept      *bool         `json:"accept,omitempty" jsonschema:"Accept or dismiss dialog (handle_dialog)"`
	TextGone    string        `json:"text_gone,omitempty" jsonschema:"Text to wait for to disappear (wait_for action)"`
	Button      string        `json:"button,omitempty" jsonschema:"Mouse button: left (default), right, middle"`
	DoubleClick bool          `json:"double_click,omitempty" jsonschema:"Double click instead of single"`
	Modifiers   []string      `json:"modifiers,omitempty" jsonschema:"Modifier keys to hold: Alt, Control, Shift, Meta"`
	Values      []string      `json:"values,omitempty" jsonschema:"Values for select_option action"`
	Depth       int           `json:"depth,omitempty" jsonschema:"Limit snapshot tree depth (0 = unlimited)"`
	Width       int           `json:"width,omitempty" jsonschema:"Viewport width for resize action"`
	Height      int           `json:"height,omitempty" jsonschema:"Viewport height for resize action"`
	Slowly      bool          `json:"slowly,omitempty" jsonschema:"Type one character at a time (type_text)"`
	Submit      bool          `json:"submit,omitempty" jsonschema:"Press Enter after typing (type_text)"`
	Fields      []FormField   `json:"fields,omitempty" jsonschema:"Fields for fill_form batch action"`
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

// FormField is a single field for the fill_form batch action.
type FormField struct {
	Selector string `json:"selector"`
	Value    string `json:"value"`
	Type     string `json:"type,omitempty"` // textbox (default), checkbox, combobox
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
// When stealthMode is true, actions that would trigger Runtime.callFunctionOn
// are routed through cdputil using pure CDP DOM/Input methods instead.
func ExecuteAction( //nolint:cyclop // dispatch switch — complexity inherent
	ctx context.Context, page *rod.Page, a Action, cursor *humanize.Cursor, logs *LogCollector, stealthMode bool,
) ActionResult {
	var (
		data any
		err  error
	)

	switch a.Type {
	case "click":
		if stealthMode {
			err = doClickStealth(ctx, page, a)
		} else if a.Humanize && cursor != nil {
			err = doClickHumanized(ctx, page, a.Selector, cursor)
		} else {
			err = doClick(ctx, page, a)
		}
	case "type_text":
		if stealthMode || a.Slowly {
			err = doTypeTextCDP(ctx, page, a.Selector, a.Text, a.Submit)
		} else if a.Humanize && cursor != nil {
			err = doTypeTextHumanized(ctx, page, a.Selector, a.Text, cursor)
		} else {
			err = doTypeText(ctx, page, a.Selector, a.Text, a.Slowly, a.Submit)
		}
	case "wait_for":
		waitCtx := ctx
		if a.TimeoutMs > 0 {
			var cancel context.CancelFunc
			waitCtx, cancel = context.WithTimeout(ctx, time.Duration(a.TimeoutMs)*time.Millisecond)
			defer cancel()
		}
		switch {
		case a.Text != "":
			err = doWaitForText(waitCtx, page, a.Text)
		case a.TextGone != "":
			err = doWaitForTextGone(waitCtx, page, a.TextGone)
		case a.WaitMs > 0 && a.Selector == "":
			err = doSleep(waitCtx, a.WaitMs)
		default:
			if stealthMode {
				err = doWaitForStealth(waitCtx, page, a.Selector)
			} else {
				err = doWaitFor(waitCtx, page, a.Selector)
			}
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
	case "sleep", "wait":
		err = doSleep(ctx, a.WaitMs)
	case "navigate":
		err = doNavigate(ctx, page, a.URL)
	case "set_cookies":
		err = doSetCookies(page, a.Cookies)
	case "snapshot":
		data, err = doSnapshot(page, a.Depth)
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
		if stealthMode {
			err = doHoverStealth(ctx, page, a.Selector)
		} else if a.Humanize && cursor != nil {
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
		if stealthMode && a.Selector != "" {
			var nodeID cdputil.NodeID
			nodeID, err = cdputil.QuerySelector(page, a.Selector)
			if err == nil {
				err = cdputil.ScrollIntoView(page, nodeID)
			}
		} else {
			err = doScroll(ctx, page, a.Selector, a.DeltaX, a.DeltaY)
		}
	case "select_option":
		err = doSelectOption(ctx, page, a.Selector, a.Values)
	case "resize":
		err = doResize(page, a.Width, a.Height)
	case "fill_form":
		if stealthMode {
			err = doFillFormStealth(ctx, page, a.Fields)
		} else {
			err = doFillForm(ctx, page, a.Fields)
		}
	default:
		err = fmt.Errorf("unknown action type: %q", a.Type)
	}

	if err != nil {
		return ActionResult{Action: a.Type, Ok: false, Error: err.Error()}
	}
	return ActionResult{Action: a.Type, Ok: true, Data: data}
}
