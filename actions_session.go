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

func execGetCookies(dc dispatchContext, a Action) (any, error) {
	cookies, err := doGetCookies(dc.page)
	if err != nil {
		return nil, err
	}
	if a.Limit > 0 && a.Limit < len(cookies) {
		cookies = cookies[len(cookies)-a.Limit:]
	}
	return cookies, nil
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

func execGetLogs(dc dispatchContext, a Action) (any, error) {
	netLimit := defaultNetworkLimit
	conLimit := defaultConsoleLimit
	if a.Limit > 0 {
		netLimit = a.Limit
		conLimit = a.Limit
	}

	if dc.logs == nil {
		return map[string]any{"network": []NetworkEntry{}, "console": []ConsoleEntry{}}, nil
	}

	net, con := dc.logs.Collect()
	net = lastN(net, netLimit)
	con = lastN(con, conLimit)

	// Return compact network entries: drop body_size, truncate URL.
	type compactNetwork struct {
		Method   string `json:"method"`
		URL      string `json:"url"`
		Status   int    `json:"status,omitempty"`
		MimeType string `json:"mime_type,omitempty"`
		Error    string `json:"error,omitempty"`
	}
	compact := make([]compactNetwork, len(net))
	for i, e := range net {
		compact[i] = compactNetwork{
			Method:   e.Method,
			URL:      truncateURL(e.URL),
			Status:   e.Status,
			MimeType: e.MimeType,
			Error:    e.Error,
		}
	}

	return map[string]any{"network": compact, "console": con}, nil
}

func execWarmup(dc dispatchContext, a Action) (any, error) {
	waitMs := a.WaitMs
	if waitMs <= 0 {
		waitMs = defaultWarmupMs
	}
	count, err := doWarmup(dc.ctx, dc.page, waitMs, dc.cursor)
	return count, err
}
