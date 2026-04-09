package browser

// actions_click.go — executors for click, hover, go_back actions.

func init() {
	registerAction("click", execClick)
	registerAction("hover", execHover)
	registerAction("go_back", execGoBack)
}

func execClick(dc dispatchContext, a Action) (any, error) {
	if dc.stealthMode {
		return nil, doClickStealth(dc.ctx, dc.page, a)
	}
	if a.Humanize && dc.cursor != nil {
		return nil, doClickHumanized(dc.ctx, dc.page, a.Selector, dc.cursor)
	}
	return nil, doClick(dc.ctx, dc.page, a)
}

func execHover(dc dispatchContext, a Action) (any, error) {
	if dc.stealthMode {
		return nil, doHoverStealth(dc.ctx, dc.page, a.Selector)
	}
	if a.Humanize && dc.cursor != nil {
		return nil, doHoverHumanized(dc.ctx, dc.page, a.Selector, dc.cursor)
	}
	return nil, doHover(dc.ctx, dc.page, a.Selector)
}

func execGoBack(dc dispatchContext, _ Action) (any, error) {
	return nil, doGoBack(dc.page)
}
