package browser

// actions_eval.go — executors for evaluate, eval_on_new_document, screenshot, snapshot actions.

func init() {
	registerAction("evaluate", execEvaluate)
	registerAction("eval_on_new_document", execEvalOnNewDocument)
	registerAction("screenshot", execScreenshot)
	registerAction("snapshot", execSnapshot)
}

func execEvaluate(dc dispatchContext, a Action) (any, error) {
	script := a.Script
	if script == "" {
		script = a.JS
	}
	return doEvaluate(dc.page, script)
}

func execEvalOnNewDocument(dc dispatchContext, a Action) (any, error) {
	script := a.Script
	if script == "" {
		script = a.JS
	}
	_, err := dc.page.EvalOnNewDocument(script)
	return nil, err
}

func execScreenshot(dc dispatchContext, a Action) (any, error) {
	// format="image" or "full" → actual JPEG screenshot.
	// Default (no format) → return snapshot text instead — saves ~150K tokens.
	switch a.Format {
	case "image":
		return doScreenshot(dc.page, false)
	case "full":
		return doScreenshot(dc.page, true)
	default:
		return doSnapshot(dc.page, a.Depth, "", a.Filter, a.Selector, dc.refMap)
	}
}

func execSnapshot(dc dispatchContext, a Action) (any, error) {
	return doSnapshot(dc.page, a.Depth, a.Format, a.Filter, a.Selector, dc.refMap)
}
