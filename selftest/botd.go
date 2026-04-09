package selftest

import (
	"context"

	"github.com/go-rod/rod"
)

// extractBotD reads the bot detection verdict from https://fingerprintjs.github.io/BotD/main/
func extractBotD(ctx context.Context, page *rod.Page) (TargetResult, error) {
	return stubExtractor(ctx, page)
}
