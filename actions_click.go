package browser

// actions_click.go — executors for click, hover, go_back actions.

func init() {
	registerAction("click", execClick)
	registerAction("hover", execHover)
	registerAction("go_back", execGoBack)
}

func execClick(dc dispatchContext, a Action) (any, error) {
	if dc.stealthMode {
		return nil, withRetry(dc.ctx, func() error {
			return doClickStealth(dc.ctx, dc.page, a, dc.refMap)
		})
	}
	if a.Humanize && dc.cursor != nil {
		return nil, withRetry(dc.ctx, func() error {
			return doClickHumanized(dc.ctx, dc.page, a.Selector, dc.cursor)
		})
	}
	return nil, withRetry(dc.ctx, func() error {
		return doClick(dc.ctx, dc.page, a, dc.refMap)
	})
}

func execHover(dc dispatchContext, a Action) (any, error) {
	if dc.stealthMode {
		return nil, doHoverStealth(dc.ctx, dc.page, a.Selector, dc.refMap)
	}
	if a.Humanize && dc.cursor != nil {
		return nil, doHoverHumanized(dc.ctx, dc.page, a.Selector, dc.cursor)
	}
	return nil, doHover(dc.ctx, dc.page, a.Selector, dc.refMap)
}

func execGoBack(dc dispatchContext, _ Action) (any, error) {
	return nil, doGoBack(dc.page)
}
