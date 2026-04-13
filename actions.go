package browser

import (
	"context"
	"fmt"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
)

// Action describes a single Chrome interaction step.
type Action struct {
	Type          string        `json:"type" jsonschema:"Action type: click, type_text, wait_for (CSS selector, text, text_gone, or wait_ms for time-based wait), snapshot (accessibility tree — best for AI), screenshot (returns snapshot text by default; use format='image' for JPEG, format='full' for full-page JPEG), evaluate (any JS expression), eval_on_new_document, press (supports F1-F12), sleep/wait, navigate, set_cookies, handle_dialog, get_cookies, destroy_session, hover, go_back, get_logs, warmup, scroll, select_option (select dropdown values by text), get_storage, set_storage, clear_storage. Selectors support CSS, text=, xpath=, ref= prefixes. Use ref=eN after a snapshot action to click/type elements by their ref id. screenshot without format returns snapshot text (saves tokens)."`
	Selector      string        `json:"selector,omitempty" jsonschema:"Element selector for click/type_text/wait_for/hover/scroll. Supports: CSS (#id, .class), text=Text, xpath=//div, ref=eN (from snapshot). Best practice: snapshot first, then use ref=eN for precise targeting."`
	Text          string        `json:"text,omitempty" jsonschema:"Text to type (type_text) or prompt response (handle_dialog)"`
	Script        string        `json:"script,omitempty" jsonschema:"JavaScript code for evaluate/eval_on_new_document"`
	JS            string        `json:"js,omitempty" jsonschema:"Alias for script"`
	Key           string        `json:"key,omitempty" jsonschema:"Key name for press action (Enter/Tab/Escape/etc)"`
	URL           string        `json:"url,omitempty" jsonschema:"URL for navigate action"`
	Humanize      bool          `json:"humanize,omitempty" jsonschema:"Use human-like mouse movement for click/type_text/hover"`
	WaitMs        int           `json:"wait_ms,omitempty" jsonschema:"Milliseconds to wait (sleep) or warmup duration"`
	TimeoutMs     int           `json:"timeout_ms,omitempty" jsonschema:"Timeout in ms for wait_for action"`
	Format        string        `json:"format,omitempty"`
	Cookies       []CookieInput `json:"cookies,omitempty" jsonschema:"Cookies for set_cookies action"`
	DeltaX        float64       `json:"delta_x,omitempty" jsonschema:"Horizontal scroll delta for scroll action"`
	DeltaY        float64       `json:"delta_y,omitempty" jsonschema:"Vertical scroll delta for scroll action"`
	Accept        *bool         `json:"accept,omitempty" jsonschema:"Accept or dismiss dialog (handle_dialog)"`
	TextGone      string        `json:"text_gone,omitempty" jsonschema:"Text to wait for to disappear (wait_for action)"`
	Button        string        `json:"button,omitempty" jsonschema:"Mouse button: left (default), right, middle"`
	DoubleClick   bool          `json:"double_click,omitempty" jsonschema:"Double click instead of single"`
	Modifiers     []string      `json:"modifiers,omitempty" jsonschema:"Modifier keys to hold: Alt, Control, Shift, Meta"`
	Values        []string      `json:"values,omitempty" jsonschema:"Values for select_option action"`
	Depth         int           `json:"depth,omitempty" jsonschema:"Limit snapshot tree depth (0 = unlimited)"`
	Filter        string        `json:"filter,omitempty" jsonschema:"Snapshot filter mode: interactive (keep actionable elements + ancestors), forms (keep form subtrees), main (keep main content), text (exclude nav/banner/contentinfo)"`
	URLContains   string        `json:"url_contains,omitempty" jsonschema:"Keep only nodes whose URL contains this substring (links/iframes)"`
	Width         int           `json:"width,omitempty" jsonschema:"Viewport width for resize action"`
	Height        int           `json:"height,omitempty" jsonschema:"Viewport height for resize action"`
	Slowly        bool          `json:"slowly,omitempty" jsonschema:"Type one character at a time (type_text)"`
	Submit        bool          `json:"submit,omitempty" jsonschema:"Press Enter after typing (type_text)"`
	Fields        []FormField   `json:"fields,omitempty" jsonschema:"Fields for fill_form batch action"`
	Cookie        string        `json:"cookie,omitempty" jsonschema:"Cookie name to wait for (wait_for action — polls until cookie appears)"`
	Limit         int           `json:"limit,omitempty" jsonschema:"Max entries to return for get_logs (default: 30 network / 20 console) and get_cookies"`
	StorageType   string        `json:"storage_type,omitempty" jsonschema:"Storage type: local (default) or session"`
	Goal          string        `json:"goal,omitempty" jsonschema:"Goal description for plan_actions (what you want to accomplish on this page)"`
	FrameSelector string        `json:"frame_selector,omitempty" jsonschema:"Target iframe for this action. CSS selector (iframe.payment) or url=pattern (url=payments.audienceview.com). Auto-waits for iframe to load (retries until timeout). type_text auto-uses CDP events. Pattern: snapshot with frame_selector to get ref=eN inside iframe, then type_text/click with same frame_selector and ref=eN."`
	SkipOnError   bool          `json:"skip_on_error,omitempty" jsonschema:"When true, a failure of this action does not abort the action chain and does not set status=error on the response."`
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

// dispatchContext bundles the per-call dependencies passed to every action executor.
type dispatchContext struct {
	ctx         context.Context
	page        *rod.Page
	cursor      *humanize.Cursor
	logs        *LogCollector
	stealthMode bool
	refMap      *RefMap
}

// actionExecutor is a function that runs a single action and returns optional data.
type actionExecutor func(dc dispatchContext, a Action) (any, error)

//nolint:gochecknoglobals // static dispatch table, populated by init() calls in action files
var actionRegistry = map[string]actionExecutor{}

// registerAction adds an executor to the dispatch table.
// Called from init() functions in focused action files.
func registerAction(actionType string, exec actionExecutor) {
	actionRegistry[actionType] = exec
}

// ExecuteAction dispatches to the registered executor for a.Type.
// When cursor is non-nil and a.Humanize is true, humanized variants are used
// for click, type_text, and hover actions.
// When stealthMode is true, actions that would trigger Runtime.callFunctionOn
// are routed through cdputil using pure CDP DOM/Input methods instead.
func ExecuteAction(
	ctx context.Context, page *rod.Page, a Action, cursor *humanize.Cursor, logs *LogCollector, stealthMode bool, refMap *RefMap,
) ActionResult {
	// type_into_frame handles its own iframe targeting via clickIframeArea.
	// Do NOT route through resolveFrame: it fails on OOP cross-origin iframes
	// and the fallback only covers type_text.
	if a.Type == "type_into_frame" {
		exec, ok := actionRegistry[a.Type]
		if !ok {
			return ActionResult{Action: a.Type, Ok: false, Error: "type_into_frame not registered"}
		}
		dc := dispatchContext{
			ctx:         ctx,
			page:        page,
			cursor:      cursor,
			logs:        logs,
			stealthMode: stealthMode,
			refMap:      refMap,
		}
		data, err := exec(dc, a)
		if err != nil {
			return ActionResult{Action: a.Type, Ok: false, Error: err.Error()}
		}
		return ActionResult{Action: a.Type, Ok: true, Data: data}
	}

	// Frame targeting: switch context to iframe if specified.
	targetPage := page
	if a.FrameSelector != "" {
		framePage, err := resolveFrame(ctx, page, a.FrameSelector)
		if err != nil {
			// OOP cross-origin iframe fallback: click iframe area to focus,
			// then type via Input.dispatchKeyEvent on the main page.
			if a.Type == "type_text" {
				if clickErr := clickIframeArea(ctx, page, a.FrameSelector); clickErr != nil {
					return ActionResult{Action: a.Type, Ok: false, Error: fmt.Sprintf("frame fallback: %v (original: %v)", clickErr, err)}
				}
				return execTypeViaKeyboard(ctx, page, a)
			}
			return ActionResult{Action: a.Type, Ok: false, Error: err.Error()}
		}
		targetPage = framePage
		// Disable cursor humanization inside frames — coordinates are relative
		// to the parent viewport, not the frame viewport.
		cursor = nil

		// Force CDP typing inside frames — Runtime.callFunctionOn fails cross-origin.
		if a.Type == "type_text" || a.Type == "fill_form" {
			a.Slowly = true
		}
	}

	exec, ok := actionRegistry[a.Type]
	if !ok {
		return ActionResult{Action: a.Type, Ok: false, Error: fmt.Sprintf("unknown action type: %q", a.Type)}
	}

	dc := dispatchContext{
		ctx:         ctx,
		page:        targetPage,
		cursor:      cursor,
		logs:        logs,
		stealthMode: stealthMode,
		refMap:      refMap,
	}

	data, err := exec(dc, a)
	if err != nil {
		return ActionResult{Action: a.Type, Ok: false, Error: err.Error()}
	}
	return ActionResult{Action: a.Type, Ok: true, Data: data}
}
