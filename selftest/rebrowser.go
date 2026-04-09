package selftest

import (
	"context"

	"github.com/go-rod/rod"
)

// extractRebrowser reads window.botDetectorResults from https://bot-detector.rebrowser.net/
func extractRebrowser(ctx context.Context, page *rod.Page) (TargetResult, error) {
	return stubExtractor(ctx, page)
}
