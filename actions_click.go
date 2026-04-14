package browser

// actions_click.go — executors for click, hover, go_back actions.

func init() {
	registerAction("click", execClick)
	registerAction("hover", execHover)
	registerAction("go_back", execGoBack)
}

func execClick(dc dispatchContext, a Action) (any, error) {
	ctx, cancel := dc.withActionTimeout(a.TimeoutMs)
	defer cancel()
	if dc.stealthMode {
		return nil, withRetry(ctx, func() error {
			return doClickStealth(ctx, dc.page, a, dc.refMap)
		})
	}
	if a.Humanize && dc.cursor != nil {
		return nil, withRetry(ctx, func() error {
			return doClickHumanized(ctx, dc.page, a.Selector, dc.cursor)
		})
	}
	return nil, withRetry(ctx, func() error {
		return doClick(ctx, dc.page, a, dc.refMap)
	})
}

func execHover(dc dispatchContext, a Action) (any, error) {
	ctx, cancel := dc.withActionTimeout(a.TimeoutMs)
	defer cancel()
	if dc.stealthMode {
		return nil, doHoverStealth(ctx, dc.page, a.Selector, dc.refMap)
	}
	if a.Humanize && dc.cursor != nil {
		return nil, doHoverHumanized(ctx, dc.page, a.Selector, dc.cursor)
	}
	return nil, doHover(ctx, dc.page, a.Selector, dc.refMap)
}

func execGoBack(dc dispatchContext, _ Action) (any, error) {
	return nil, doGoBack(dc.page)
}
