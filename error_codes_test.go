package browser

import (
	"errors"
	"testing"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		in   string
		want ErrorCode
	}{
		{"context canceled", ErrCodeContextCanceled},
		{"navigation timeout after 30s", ErrCodeNavigationTimeout},
		{"action timed out", ErrCodeActionTimeout},
		{"element not found: #foo", ErrCodeSelectorNotFound},
		// Selector hunt that hit a deadline — domain signal wins over context.
		{`click: find "#missing": context deadline exceeded`, ErrCodeSelectorNotFound},
		{`wait_for: find ".thing": timed out`, ErrCodeSelectorNotFound},
		// Plain deadline without selector context still reads as timeout.
		{"context deadline exceeded", ErrCodeActionTimeout},
		// Navigation deadline — still nav timeout even with new ordering.
		{"navigation: context deadline exceeded", ErrCodeNavigationTimeout},
		{"element not visible", ErrCodeElementNotVisible},
		{"target has crashed", ErrCodeTargetCrashed},
		{"frame iframe#pay not found", ErrCodeFrameNotFound},
		{"recaptcha challenge detected", ErrCodeCaptchaDetected},
		{"ERR_NAME_NOT_RESOLVED", ErrCodeNetworkError},
		{"CDP method unsupported", ErrCodeCdpError},
		// JS exceptions from evaluate / Uncaught runtime errors.
		{`evaluate: Uncaught`, ErrCodeJsException},
		{`evaluate: Uncaught ReferenceError: foo is not defined`, ErrCodeJsException},
		// Click covered-by-overlay — typed path uses sentinel; string fallback
		// handles rod's raw `element covered by: <div.modal>` text.
		{"click: element covered by: <div class=modal>", ErrCodeElementCovered},
		{"something weird happened", ErrCodeUnknown},
	}
	for _, c := range cases {
		got := ClassifyError(errors.New(c.in))
		if got != c.want {
			t.Errorf("ClassifyError(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if ClassifyError(nil) != "" {
		t.Error("nil error should classify to empty string")
	}
}
