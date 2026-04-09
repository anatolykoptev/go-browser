package selftest

import (
	"context"

	"github.com/go-rod/rod"
)

// extractCreepJS extracts trust score, lies, and per-section hashes from
// https://abrahamjuliot.github.io/creepjs/
func extractCreepJS(ctx context.Context, page *rod.Page) (TargetResult, error) {
	return stubExtractor(ctx, page)
}
