package browser

// actions_type.go — executors for type_text, press, fill_form, select_option actions.

func init() {
	registerAction("type_text", execTypeText)
	registerAction("press", execPress)
	registerAction("fill_form", execFillForm)
	registerAction("select_option", execSelectOption)
}

func execTypeText(dc dispatchContext, a Action) (any, error) {
	if dc.stealthMode || a.Slowly {
		return nil, doTypeTextCDP(dc.ctx, dc.page, a.Selector, a.Text, a.Submit)
	}
	if a.Humanize && dc.cursor != nil {
		return nil, doTypeTextHumanized(dc.ctx, dc.page, a.Selector, a.Text, dc.cursor)
	}
	return nil, doTypeText(dc.ctx, dc.page, a.Selector, a.Text, a.Slowly, a.Submit)
}

func execPress(dc dispatchContext, a Action) (any, error) {
	return nil, doPress(dc.page, a.Key)
}

func execFillForm(dc dispatchContext, a Action) (any, error) {
	if dc.stealthMode {
		return nil, doFillFormStealth(dc.ctx, dc.page, a.Fields)
	}
	return nil, doFillForm(dc.ctx, dc.page, a.Fields)
}

func execSelectOption(dc dispatchContext, a Action) (any, error) {
	return nil, doSelectOption(dc.ctx, dc.page, a.Selector, a.Values)
}
