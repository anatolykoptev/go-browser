package selftest

import (
	"context"

	"github.com/go-rod/rod"
)

// extractWebRTCLeak checks for IP leaks at https://browserleaks.com/webrtc
func extractWebRTCLeak(ctx context.Context, page *rod.Page) (TargetResult, error) {
	return stubExtractor(ctx, page)
}
