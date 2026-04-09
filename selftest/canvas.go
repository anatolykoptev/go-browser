package selftest

import (
	"context"

	"github.com/go-rod/rod"
)

// extractCanvas extracts canvas fingerprint hash from https://browserleaks.com/canvas
func extractCanvas(ctx context.Context, page *rod.Page) (TargetResult, error) {
	return stubExtractor(ctx, page)
}
