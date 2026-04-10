package humanize

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/go-rod/rod"
)

const (
	focusActiveMinSec = 45
	focusActiveMaxSec = 120
	focusHiddenMinSec = 5
	focusHiddenMaxSec = 25
)

// FocusCycle periodically simulates tab switching by toggling document
// visibility state and dispatching blur/focus events.
type FocusCycle struct {
	cancel context.CancelFunc
}

// Stop terminates the focus cycle goroutine.
func (fc *FocusCycle) Stop() {
	fc.cancel()
}

// StartFocusCycle periodically simulates tab switching.
// Active period: 45-120s, hidden period: 5-25s.
// During hidden: dispatches blur + visibilitychange events.
// Returns a FocusCycle whose Stop method terminates the loop.
func StartFocusCycle(ctx context.Context, page *rod.Page) *FocusCycle {
	ctx, cancel := context.WithCancel(ctx)
	fc := &FocusCycle{cancel: cancel}

	go func() {
		for {
			// Active phase: wait 45-120s before going hidden.
			activeSec := focusActiveMinSec + rand.IntN(focusActiveMaxSec-focusActiveMinSec)
			if !sleepFocus(ctx, time.Duration(activeSec)*time.Second) {
				return
			}

			// Go hidden: dispatch blur + visibilitychange.
			hideTab(page)

			// Hidden phase: 5-25s with no mouse/keyboard events.
			hiddenSec := focusHiddenMinSec + rand.IntN(focusHiddenMaxSec-focusHiddenMinSec)
			if !sleepFocus(ctx, time.Duration(hiddenSec)*time.Second) {
				// Restore before exit even when cancelled.
				showTab(page)
				return
			}

			// Restore: reverse visibility override + dispatch focus.
			showTab(page)
		}
	}()

	return fc
}

// hideTab dispatches blur and sets visibilityState to "hidden".
func hideTab(page *rod.Page) {
	_, _ = page.Eval(`() => {
		document.dispatchEvent(new Event('blur'));
		Object.defineProperty(document, 'visibilityState', {
			value: 'hidden',
			configurable: true,
			writable: true,
		});
		Object.defineProperty(document, 'hidden', {
			value: true,
			configurable: true,
			writable: true,
		});
		document.dispatchEvent(new Event('visibilitychange'));
	}`)
}

// showTab restores visibilityState to "visible" and dispatches focus.
func showTab(page *rod.Page) {
	_, _ = page.Eval(`() => {
		Object.defineProperty(document, 'visibilityState', {
			value: 'visible',
			configurable: true,
			writable: true,
		});
		Object.defineProperty(document, 'hidden', {
			value: false,
			configurable: true,
			writable: true,
		});
		document.dispatchEvent(new Event('visibilitychange'));
		document.dispatchEvent(new Event('focus'));
		window.dispatchEvent(new Event('focus'));
	}`)
}

// sleepFocus sleeps for d and returns true, or returns false if ctx is done.
func sleepFocus(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
