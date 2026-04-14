package browser

// actions_type.go — executors for type_text, press, fill_form, select_option actions.

func init() {
	registerAction("type_text", execTypeText)
	registerAction("press", execPress)
	registerAction("fill_form", execFillForm)
	registerAction("select_option", execSelectOption)
}

func execTypeText(dc dispatchContext, a Action) (any, error) {
	ctx, cancel := dc.withActionTimeout(a.TimeoutMs)
	defer cancel()
	if dc.stealthMode || a.Slowly {
		return nil, withRetry(ctx, func() error {
			return doTypeTextCDP(ctx, dc.page, a.Selector, a.Text, a.Submit, dc.refMap)
		})
	}
	if a.Humanize && dc.cursor != nil {
		return nil, withRetry(ctx, func() error {
			return doTypeTextHumanized(ctx, dc.page, a.Selector, a.Text, dc.cursor)
		})
	}
	return nil, withRetry(ctx, func() error {
		return doTypeText(ctx, dc.page, a.Selector, a.Text, a.Slowly, a.Submit, dc.refMap)
	})
}

func execPress(dc dispatchContext, a Action) (any, error) {
	return nil, doPress(dc.page, a.Key, a.Modifiers)
}

func execFillForm(dc dispatchContext, a Action) (any, error) {
	if dc.stealthMode {
		return nil, doFillFormStealth(dc.ctx, dc.page, a.Fields, dc.refMap)
	}
	return nil, doFillForm(dc.ctx, dc.page, a.Fields, dc.refMap)
}

func execSelectOption(dc dispatchContext, a Action) (any, error) {
	return nil, doSelectOption(dc.ctx, dc.page, a.Selector, a.Values, dc.refMap)
}
