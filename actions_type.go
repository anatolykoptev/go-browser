package browser

// actions_type.go — executors for type_text, press, fill_form, select_option actions.

func init() {
	registerAction("type_text", execTypeText)
	registerAction("press", execPress)
	registerAction("fill_form", execFillForm)
	registerAction("select_option", execSelectOption)
	registerAction("select_all", execSelectAll)
	registerAction("insert_text_input_event", execInsertTextInputEvent)
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

// execSelectAll selects all text in the focused element or specified selector.
// Uses JavaScript Selection API to bypass TipTap/ProseMirror interceptors.
func execSelectAll(dc dispatchContext, a Action) (any, error) {
	return nil, doSelectAll(dc.page, a.Selector, dc.refMap)
}

// execInsertTextInputEvent dispatches synthetic beforeinput+input events with
// inputType="insertReplacementText" — the spec-blessed way to drive TipTap /
// ProseMirror contenteditable editors. Returns the diagnostic string so the
// caller can see "cancelled=?" and a live readout of the field contents.
func execInsertTextInputEvent(dc dispatchContext, a Action) (any, error) {
	return doInsertTextInputEvent(dc.page, a.Selector, a.Text)
}
