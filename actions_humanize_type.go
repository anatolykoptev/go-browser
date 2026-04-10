package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// doTypeTextHumanized types text character by character with human-like delays,
// dispatching real keydown/char/keyup CDP events so keyboard listeners see authentic input.
func doTypeTextHumanized(
	ctx context.Context, page *rod.Page, selector, text string, cursor *humanize.Cursor,
) error {
	if err := doClickHumanized(ctx, page, selector, cursor); err != nil {
		return fmt.Errorf("type_text: focus: %w", err)
	}

	delays := humanize.TypingDelays(text)
	for i, ch := range text {
		char := string(ch)
		ci := humanize.LookupChar(ch)
		code := ci.Code
		vk := ci.VK

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeRawKeyDown,
			Key:                   char,
			Code:                  code,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		// Key dwell time (T4 TMX behavioral biometric): hold key for 40-120ms
		// before firing char and keyUp. Real humans don't release instantly.
		sleepCtx(ctx, time.Duration(humanize.KeyDwellTime())*time.Millisecond)

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeChar,
			Text:                  char,
			UnmodifiedText:        char,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeKeyUp,
			Key:                   char,
			Code:                  code,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		// Word boundary pause: extra 300-500ms at spaces (15% chance)
		// simulating inter-word cognitive processing gap.
		if ch == ' ' {
			if pause := humanize.WordBoundaryPause(); pause > 0 {
				sleepCtx(ctx, time.Duration(pause)*time.Millisecond)
			}
		}

		// Inter-key delay (gaussian μ=120ms, σ=40ms).
		if i < len(delays) {
			sleepCtx(ctx, time.Duration(delays[i])*time.Millisecond)
		}
	}
	return nil
}
