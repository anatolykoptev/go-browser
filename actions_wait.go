package browser

import (
	"context"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod/lib/proto"
)

// actions_wait.go — executors for wait_for, sleep, wait actions.

func init() {
	registerAction("wait_for", execWaitFor)
	registerAction("wait_for_navigation", execWaitForNavigation)
	registerAction("sleep", execSleep)
	registerAction("wait", execSleep)
}

func execWaitFor(dc dispatchContext, a Action) (any, error) {
	waitCtx, cancel := dc.withActionTimeout(a.TimeoutMs)
	defer cancel()
	if dc.stealthMode && dc.cursor != nil {
		stop := startWaitDrift(waitCtx, dc)
		defer stop()
	}
	return nil, dispatchWaitFor(waitCtx, dc, a)
}

// dispatchWaitFor selects the correct wait variant based on action fields.
func dispatchWaitFor(waitCtx context.Context, dc dispatchContext, a Action) error {
	switch {
	case a.Cookie != "":
		return doWaitForCookie(waitCtx, dc.page, a.Cookie)
	case a.Text != "":
		return doWaitForText(waitCtx, dc.page, a.Text)
	case a.TextGone != "":
		return doWaitForTextGone(waitCtx, dc.page, a.TextGone)
	case a.WaitMs > 0 && a.Selector == "":
		return doSleep(waitCtx, a.WaitMs)
	default:
		if dc.stealthMode {
			return doWaitForStealth(waitCtx, dc.page, a.Selector, dc.refMap)
		}
		return doWaitFor(waitCtx, dc.page, a.Selector, dc.refMap)
	}
}

func execWaitForNavigation(dc dispatchContext, a Action) (any, error) {
	timeoutMs := 10000 // 10 seconds default
	if a.TimeoutMs > 0 {
		timeoutMs = a.TimeoutMs
	}
	// Use remaining time from deadline if available
	remaining := dc.remainingMs()
	if remaining < timeoutMs {
		timeoutMs = remaining
	}
	waitCtx, cancel := context.WithTimeout(dc.ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	return doWaitForNavigation(waitCtx, dc.page, a.URLContains, a.Selector, dc.refMap)
}

func execSleep(dc dispatchContext, a Action) (any, error) {
	waitMs := a.WaitMs
	if waitMs > 0 {
		// Use remaining time from deadline if available
		remaining := dc.remainingMs()
		if remaining < waitMs {
			waitMs = remaining
		}
	}
	if dc.stealthMode && dc.cursor != nil && waitMs > 0 {
		sleepCtx, cancel := context.WithTimeout(dc.ctx, time.Duration(waitMs)*time.Millisecond)
		defer cancel()
		stop := startWaitDrift(sleepCtx, dc)
		defer stop()
	}
	if waitMs > 0 {
		time.Sleep(time.Duration(waitMs) * time.Millisecond)
	}
	return nil, nil
}

// startWaitDrift starts idle drift for the duration of a wait/sleep action.
// Returns a stop function that must be called when the wait is done.
// The drift dispatches micro mouse-moves via CDP Input events.
func startWaitDrift(ctx context.Context, dc dispatchContext) func() {
	dispatch := func(x, y float64) error {
		return proto.InputDispatchMouseEvent{
			Type: proto.InputDispatchMouseEventTypeMouseMoved,
			X:    x,
			Y:    y,
		}.Call(dc.page)
	}
	return humanize.StartIdleDrift(ctx, dc.cursor, dispatch)
}
