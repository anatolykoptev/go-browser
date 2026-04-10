package selftest

import (
	"context"

	"github.com/go-rod/rod"
)

// Target describes a probe destination.
type Target struct {
	Key string
	URL string
}

// AllTargets is the ordered list of antibot probe targets.
var AllTargets = []Target{
	{Key: "creepjs", URL: "https://abrahamjuliot.github.io/creepjs/"},
	{Key: "sannysoft", URL: "https://bot.sannysoft.com/"},
	{Key: "rebrowser", URL: "https://bot-detector.rebrowser.net/"},
	{Key: "botd", URL: "https://fingerprintjs.github.io/BotD/main/"},
	{Key: "webrtc_leak", URL: "https://browserleaks.com/webrtc"},
	{Key: "canvas", URL: "https://browserleaks.com/canvas"},
	{Key: "incolumitas", URL: "https://bot.incolumitas.com/"},
	{Key: "browserscan", URL: "https://www.browserscan.net/bot-detection"},
}

// Extractor is a function that probes a page and returns a TargetResult.
// The page has already been navigated to the target URL.
type Extractor func(ctx context.Context, page *rod.Page) (TargetResult, error)

// Extractors maps each target key to its extractor implementation.
// Keys match AllTargets[*].Key exactly.
var Extractors = map[string]Extractor{
	"creepjs":     extractCreepJS,
	"sannysoft":   extractSannysoft,
	"rebrowser":   extractRebrowser,
	"botd":        extractBotD,
	"webrtc_leak": extractWebRTCLeak,
	"canvas":      extractCanvas,
	"incolumitas": extractIncolumitas,
	"browserscan": extractBrowserScan,
}
