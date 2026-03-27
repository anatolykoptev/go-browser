package browser

import (
	"context"
	"fmt"

	"github.com/go-rod/rod"
)

// Action describes a single Chrome interaction step.
type Action struct {
	Type     string        `json:"type"`
	Selector string        `json:"selector,omitempty"`
	Text     string        `json:"text,omitempty"`
	Script   string        `json:"script,omitempty"`
	Key      string        `json:"key,omitempty"`
	URL      string        `json:"url,omitempty"`
	Humanize bool          `json:"humanize,omitempty"`
	WaitMs   int           `json:"wait_ms,omitempty"`
	Format   string        `json:"format,omitempty"`
	Cookies  []CookieInput `json:"cookies,omitempty"`
	DeltaX   float64       `json:"delta_x,omitempty"`
	DeltaY   float64       `json:"delta_y,omitempty"`
}

// CookieInput holds cookie data for the set_cookies action.
type CookieInput struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path,omitempty"`
}

// ActionResult is the outcome of a single executed action.
type ActionResult struct {
	Action string `json:"action"`
	Ok     bool   `json:"ok"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ExecuteAction dispatches to the correct do* function based on a.Type.
// Returns an ActionResult with Ok=false and Error set on failure.
func ExecuteAction(ctx context.Context, page *rod.Page, a Action) ActionResult { //nolint:cyclop // dispatch switch — complexity inherent
	var (
		data any
		err  error
	)

	switch a.Type {
	case "click":
		err = doClick(ctx, page, a.Selector)
	case "type_text":
		err = doTypeText(ctx, page, a.Selector, a.Text)
	case "wait_for":
		err = doWaitFor(ctx, page, a.Selector)
	case "screenshot":
		data, err = doScreenshot(page)
	case "evaluate":
		data, err = doEvaluate(page, a.Script)
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
		data, err = doHandleDialog(page)
	case "hover":
		err = doHover(ctx, page, a.Selector)
	case "go_back":
		err = doGoBack(page)
	case "get_logs":
		data = doGetLogs()
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
