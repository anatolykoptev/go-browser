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
		{"element not visible", ErrCodeElementNotVisible},
		{"target has crashed", ErrCodeTargetCrashed},
		{"frame iframe#pay not found", ErrCodeFrameNotFound},
		{"recaptcha challenge detected", ErrCodeCaptchaDetected},
		{"ERR_NAME_NOT_RESOLVED", ErrCodeNetworkError},
		{"CDP method unsupported", ErrCodeCdpError},
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
