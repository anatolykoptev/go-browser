package browser

import (
	"context"
	"time"
)

// actions_wait.go — executors for wait_for, sleep, wait actions.

func init() {
	registerAction("wait_for", execWaitFor)
	registerAction("wait_for_navigation", execWaitForNavigation)
	registerAction("sleep", execSleep)
	registerAction("wait", execSleep)
}

func execWaitFor(dc dispatchContext, a Action) (any, error) {
	waitCtx := dc.ctx
	if a.TimeoutMs > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(dc.ctx, time.Duration(a.TimeoutMs)*time.Millisecond)
		defer cancel()
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
			return doWaitForStealth(waitCtx, dc.page, a.Selector)
		}
		return doWaitFor(waitCtx, dc.page, a.Selector)
	}
}

func execWaitForNavigation(dc dispatchContext, a Action) (any, error) {
	timeout := 10 * time.Second
	if a.TimeoutMs > 0 {
		timeout = time.Duration(a.TimeoutMs) * time.Millisecond
	}
	waitCtx, cancel := context.WithTimeout(dc.ctx, timeout)
	defer cancel()
	return doWaitForNavigation(waitCtx, dc.page, a.URLContains, a.Selector)
}

func execSleep(dc dispatchContext, a Action) (any, error) {
	return nil, doSleep(dc.ctx, a.WaitMs)
}
