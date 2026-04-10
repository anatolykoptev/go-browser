package selftest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const botdWaitTimeout = 30 * time.Second

// botdReadyJS returns a non-empty string once BotD has rendered its result.
const botdReadyJS = `
() => {
  // BotD renders a result element with bot/not-bot verdict
  var el = document.querySelector('.result, #result, [data-result], .bd-result');
  if (el) return el.textContent.trim();
  // Fallback: check for any verdict text
  var body = document.body ? document.body.innerText : '';
  if (body.includes('bot') || body.includes('Bot') || body.includes('human')) return body.substring(0, 100);
  return null;
}
`

// botdExtractJS extracts the BotD verdict and confidence.
const botdExtractJS = `
() => {
  // BotD exposes window.BotD or a result element
  var out = { bot: null, confidence: null, raw: '' };

  // Try DOM result panel first
  var resultEl = document.querySelector('.result, #result, [data-result]');
  if (resultEl) {
    out.raw = resultEl.textContent.trim();
    out.bot = out.raw.toLowerCase().includes('bot') && !out.raw.toLowerCase().includes('not bot');
  }

  // Try visible verdict text
  var verdictEl = document.querySelector('.verdict, [class*="verdict"], [class*="bot-result"]');
  if (verdictEl) {
    out.raw = verdictEl.textContent.trim();
    out.bot = out.raw.toLowerCase().includes('bot') && !out.raw.toLowerCase().includes('not a bot');
  }

  // Scan all text for explicit bot/human verdict
  var allText = document.body ? document.body.innerText : '';
  if (allText.toLowerCase().includes('you are a bot') || allText.toLowerCase().includes('detected as bot')) {
    out.bot = true;
  } else if (allText.toLowerCase().includes('not a bot') || allText.toLowerCase().includes('human')) {
    out.bot = false;
  }

  return JSON.stringify(out);
}
`

// extractBotD reads the bot detection verdict from https://fingerprintjs.github.io/BotD/main/
//
// Strategy: wait for BotD to render its result panel, then evaluate verdict.
// ok=true means BotD classified us as human (not bot).
func extractBotD(ctx context.Context, page *rod.Page) (TargetResult, error) {
	result := TargetResult{
		Target: "botd",
		URL:    "https://fingerprintjs.github.io/BotD/main/",
	}

	deadline := time.Now().Add(botdWaitTimeout)
	for time.Now().Before(deadline) {
		val, err := page.Eval(botdReadyJS)
		if err == nil && val != nil {
			s := strings.TrimSpace(val.Value.String())
			if s != "" && s != "null" {
				break
			}
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("botd: context cancelled while waiting")
		case <-time.After(500 * time.Millisecond):
		}
	}

	val, err := page.Eval(botdExtractJS)
	if err != nil {
		return result, fmt.Errorf("botd: eval extract: %w", err)
	}
	if val == nil || val.Value.String() == "" || val.Value.String() == "null" {
		return result, fmt.Errorf("botd: selector not found — page structure may have changed")
	}

	var verdict struct {
		Bot        *bool  `json:"bot"`
		Confidence *int   `json:"confidence"`
		Raw        string `json:"raw"`
	}
	if err := parseJSON(val.Value.String(), &verdict); err != nil {
		return result, fmt.Errorf("botd: parse result: %w", err)
	}

	isBot := verdict.Bot != nil && *verdict.Bot
	var trustScore float64
	if !isBot {
		trustScore = maxTrustScore
	}

	result.OK = !isBot
	result.TrustScore = trustScore
	result.Sections = map[string]any{
		"bot": isBot,
		"raw": verdict.Raw,
	}
	return result, nil
}
