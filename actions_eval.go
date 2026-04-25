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
	// Format selectors:
	//   "image"    → JPEG, viewport only (default token-friendly output)
	//   "full"     → JPEG, full scrollable height
	//   "png"      → PNG, viewport only (lossless, larger)
	//   "full_png" → PNG, full scrollable height
	//   ""         → snapshot text (NOT a screenshot — saves ~150K LLM tokens)
	//
	// When OutputPath is set, bytes are written to disk and the action returns
	// a {path, bytes_size, width, height, format} struct. Otherwise returns
	// base64-encoded bytes as a string (back-compat with token-streaming flows).
	opts := screenshotOptions{
		fullPage:   a.Format == "full" || a.Format == "full_png",
		pngFormat:  a.Format == "png" || a.Format == "full_png",
		outputPath: a.OutputPath,
		quality:    a.Quality,
	}
	switch a.Format {
	case "image", "full", "png", "full_png":
		return doScreenshotEx(dc.page, opts)
	default:
		return doSnapshot(dc.page, a.Depth, "", a.Filter, a.Selector, dc.refMap)
	}
}

func execSnapshot(dc dispatchContext, a Action) (any, error) {
	return doSnapshot(dc.page, a.Depth, a.Format, a.Filter, a.Selector, dc.refMap)
}
