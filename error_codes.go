package browser

import "strings"

// ErrorCode is a stable machine-readable classification for action failures.
// Keep values snake_case and append-only — agents branch on these values.
type ErrorCode string

const (
	// ErrCodeUnknown represents an uncategorized error
	ErrCodeUnknown           ErrorCode = "unknown"
	// ErrCodeSelectorNotFound represents CSS selector not found on page
	ErrCodeSelectorNotFound  ErrorCode = "selector_not_found"
	// ErrCodeElementNotVisible represents element found but not visible/interactive
	ErrCodeElementNotVisible ErrorCode = "element_not_visible"
	// ErrCodeNavigationTimeout represents navigation timeout
	ErrCodeNavigationTimeout ErrorCode = "navigation_timeout"
	// ErrCodeActionTimeout represents action timeout
	ErrCodeActionTimeout     ErrorCode = "action_timeout"
	// ErrCodeContextCanceled represents context cancellation
	ErrCodeContextCanceled   ErrorCode = "context_canceled"
	// ErrCodeTargetCrashed represents browser target crash
	ErrCodeTargetCrashed     ErrorCode = "target_crashed"
	// ErrCodeInvalidInput represents invalid input parameters
	ErrCodeInvalidInput      ErrorCode = "invalid_input"
	// ErrCodeFrameNotFound represents iframe not found
	ErrCodeFrameNotFound     ErrorCode = "frame_not_found"
	// ErrCodeCaptchaDetected represents captcha detection
	ErrCodeCaptchaDetected   ErrorCode = "captcha_detected"
	// ErrCodeNetworkError represents network-related errors
	ErrCodeNetworkError      ErrorCode = "network_error"
	// ErrCodeCdpError represents Chrome DevTools Protocol errors
	ErrCodeCdpError          ErrorCode = "cdp_error"
)

// ClassifyError maps a raw Go error to an ErrorCode based on its string and type.
// Unknown errors return ErrCodeUnknown — callers should still read .Error field.
func ClassifyError(err error) ErrorCode {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "context canceled") || strings.Contains(s, "context deadline exceeded"):
		if strings.Contains(s, "navigation") {
			return ErrCodeNavigationTimeout
		}
		return ErrCodeContextCanceled
	case strings.Contains(s, "not found") && (strings.Contains(s, "element") || strings.Contains(s, "selector")):
		return ErrCodeSelectorNotFound
	case strings.Contains(s, "not visible") || strings.Contains(s, "hidden"):
		return ErrCodeElementNotVisible
	case strings.Contains(s, "target") && strings.Contains(s, "crash"):
		return ErrCodeTargetCrashed
	case strings.Contains(s, "frame") && strings.Contains(s, "not found"):
		return ErrCodeFrameNotFound
	case strings.Contains(s, "navigation") && strings.Contains(s, "timeout"):
		return ErrCodeNavigationTimeout
	case strings.Contains(s, "timeout") || strings.Contains(s, "timed out"):
		return ErrCodeActionTimeout
	case strings.Contains(s, "captcha") || strings.Contains(s, "recaptcha") || strings.Contains(s, "cloudflare challenge"):
		return ErrCodeCaptchaDetected
	case strings.Contains(s, "network") || strings.Contains(s, "dns") || strings.Contains(s, "ERR_"):
		return ErrCodeNetworkError
	case strings.Contains(s, "CDP") || strings.Contains(s, "protocol"):
		return ErrCodeCdpError
	}
	return ErrCodeUnknown
}
