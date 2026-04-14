package browser

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinel_ErrorsIs(t *testing.T) {
	// Wrap a deadline error with our sentinel.
	raw := errors.New("context deadline exceeded")
	wrapped := fmt.Errorf("click: find \"#x\": %w", errors.Join(raw, ErrSelectorNotFound))

	if !errors.Is(wrapped, ErrSelectorNotFound) {
		t.Error("wrapped error should match ErrSelectorNotFound via errors.Is")
	}
	if code := ClassifyError(wrapped); code != ErrCodeSelectorNotFound {
		t.Errorf("ClassifyError of wrapped sentinel = %q, want %q", code, ErrCodeSelectorNotFound)
	}
}

func TestSentinel_FallbackStillWorks(t *testing.T) {
	// A plain error without sentinel still falls through to string matching.
	plain := errors.New("element not found: #foo")
	if ClassifyError(plain) != ErrCodeSelectorNotFound {
		t.Error("string fallback broken")
	}
}

func TestSentinel_AllSentinelsClassifyCorrectly(t *testing.T) {
	tests := []struct {
		sentinel error
		want     ErrorCode
	}{
		{ErrSelectorNotFound, ErrCodeSelectorNotFound},
		{ErrElementNotVisible, ErrCodeElementNotVisible},
		{ErrNavigationTimeout, ErrCodeNavigationTimeout},
		{ErrActionTimeout, ErrCodeActionTimeout},
		{ErrContextCanceled, ErrCodeContextCanceled},
		{ErrTargetCrashed, ErrCodeTargetCrashed},
		{ErrInvalidInput, ErrCodeInvalidInput},
		{ErrFrameNotFound, ErrCodeFrameNotFound},
		{ErrCaptchaDetected, ErrCodeCaptchaDetected},
		{ErrNetworkError, ErrCodeNetworkError},
		{ErrCdpError, ErrCodeCdpError},
	}

	for _, tt := range tests {
		code := ClassifyError(tt.sentinel)
		if code != tt.want {
			t.Errorf("ClassifyError(%v) = %q, want %q", tt.sentinel, code, tt.want)
		}
	}
}
