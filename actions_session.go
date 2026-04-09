package browser

// actions_session.go — executors for set_cookies, get_cookies, handle_dialog,
// destroy_session, get_logs, warmup actions.

const defaultWarmupMs = 3000

func init() {
	registerAction("set_cookies", execSetCookies)
	registerAction("get_cookies", execGetCookies)
	registerAction("handle_dialog", execHandleDialog)
	registerAction("destroy_session", execDestroySession)
	registerAction("get_logs", execGetLogs)
	registerAction("warmup", execWarmup)
}

func execSetCookies(dc dispatchContext, a Action) (any, error) {
	return nil, doSetCookies(dc.page, a.Cookies)
}

func execGetCookies(dc dispatchContext, _ Action) (any, error) {
	return doGetCookies(dc.page)
}

func execHandleDialog(dc dispatchContext, a Action) (any, error) {
	accept := true
	if a.Accept != nil {
		accept = *a.Accept
	}
	return doHandleDialog(dc.page, accept, a.Text)
}

// execDestroySession is a no-op: session lifecycle is managed by the HTTP handler.
func execDestroySession(_ dispatchContext, _ Action) (any, error) {
	return nil, nil
}

func execGetLogs(dc dispatchContext, _ Action) (any, error) {
	if dc.logs != nil {
		net, con := dc.logs.Collect()
		return map[string]any{"network": net, "console": con}, nil
	}
	return map[string]any{"network": []NetworkEntry{}, "console": []ConsoleEntry{}}, nil
}

func execWarmup(dc dispatchContext, a Action) (any, error) {
	waitMs := a.WaitMs
	if waitMs <= 0 {
		waitMs = defaultWarmupMs
	}
	count, err := doWarmup(dc.ctx, dc.page, waitMs, dc.cursor)
	return count, err
}
