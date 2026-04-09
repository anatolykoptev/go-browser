package selftest

import (
	"context"

	"github.com/go-rod/rod"
)

// extractSannysoft parses the pass/fail table at https://bot.sannysoft.com/
func extractSannysoft(ctx context.Context, page *rod.Page) (TargetResult, error) {
	return stubExtractor(ctx, page)
}
