package browser

import "strings"

// ErrorCode is a stable machine-readable classification for action failures.
// Keep values snake_case and append-only — agents branch on these values.
type ErrorCode string

const (
	// ErrCodeUnknown represents an uncategorized error
	ErrCodeUnknown ErrorCode = "unknown"
	// ErrCodeSelectorNotFound represents CSS selector not found on page
	ErrCodeSelectorNotFound ErrorCode = "selector_not_found"
	// ErrCodeElementNotVisible represents element found but not visible/interactive
	ErrCodeElementNotVisible ErrorCode = "element_not_visible"
	// ErrCodeNavigationTimeout represents navigation timeout
	ErrCodeNavigationTimeout ErrorCode = "navigation_timeout"
	// ErrCodeActionTimeout represents action timeout
	ErrCodeActionTimeout ErrorCode = "action_timeout"
	// ErrCodeContextCanceled represents context cancellation
	ErrCodeContextCanceled ErrorCode = "context_canceled"
	// ErrCodeTargetCrashed represents browser target crash
	ErrCodeTargetCrashed ErrorCode = "target_crashed"
	// ErrCodeInvalidInput represents invalid input parameters
	ErrCodeInvalidInput ErrorCode = "invalid_input"
	// ErrCodeFrameNotFound represents iframe not found
	ErrCodeFrameNotFound ErrorCode = "frame_not_found"
	// ErrCodeCaptchaDetected represents captcha detection
	ErrCodeCaptchaDetected ErrorCode = "captcha_detected"
	// ErrCodeNetworkError represents network-related errors
	ErrCodeNetworkError ErrorCode = "network_error"
	// ErrCodeCdpError represents Chrome DevTools Protocol errors
	ErrCodeCdpError ErrorCode = "cdp_error"
)

// ClassifyError maps a raw Go error to an ErrorCode based on its string.
// Order matters: action-specific signals (e.g. "find <sel>: deadline") are checked
// before generic context signals, so a timeout inside a selector hunt is reported
// as selector_not_found, not context_canceled.
// Unknown errors return ErrCodeUnknown — callers should still read .Error field.
func ClassifyError(err error) ErrorCode {
	if err == nil {
		return ""
	}
	s := err.Error()
	isTimeout := strings.Contains(s, "timeout") ||
		strings.Contains(s, "timed out") ||
		strings.Contains(s, "context deadline exceeded")

	switch {
	// --- 1. Strong domain signals — these win over generic context cancel ---
	case strings.Contains(s, "captcha") ||
		strings.Contains(s, "recaptcha") ||
		strings.Contains(s, "cloudflare challenge"):
		return ErrCodeCaptchaDetected
	case strings.Contains(s, "target") && strings.Contains(s, "crash"):
		return ErrCodeTargetCrashed
	case strings.Contains(s, "frame") && strings.Contains(s, "not found"):
		return ErrCodeFrameNotFound
	// A timeout while looking up a selector reads as "selector never appeared".
	// Matches both "element not found: #foo" and `click: find "#foo": deadline`.
	case (strings.Contains(s, "not found") &&
		(strings.Contains(s, "element") || strings.Contains(s, "selector"))) ||
		(strings.Contains(s, "find ") && isTimeout):
		return ErrCodeSelectorNotFound
	case strings.Contains(s, "not visible") || strings.Contains(s, "hidden"):
		return ErrCodeElementNotVisible
	case strings.Contains(s, "navigation") && isTimeout:
		return ErrCodeNavigationTimeout

	// --- 2. Generic timeouts and context cancels, after domain checks ---
	case strings.Contains(s, "context canceled"):
		return ErrCodeContextCanceled
	case isTimeout:
		return ErrCodeActionTimeout

	// --- 3. Transport-level errors, lowest priority ---
	case strings.Contains(s, "ERR_") ||
		strings.Contains(s, "network") ||
		strings.Contains(s, "dns"):
		return ErrCodeNetworkError
	case strings.Contains(s, "CDP") || strings.Contains(s, "protocol"):
		return ErrCodeCdpError
	}
	return ErrCodeUnknown
}
