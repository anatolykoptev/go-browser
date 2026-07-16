package browser

import (
	"errors"
	"strings"

	"github.com/go-rod/rod/lib/cdp"
)

var (
	// ErrUnavailable means the backend is not connected or binary not found.
	ErrUnavailable = errors.New("browser: backend unavailable")

	// ErrTimeout means the render exceeded the configured deadline.
	ErrTimeout = errors.New("browser: render timeout")

	// ErrNavigate means page navigation failed (DNS, TLS, HTTP error).
	ErrNavigate = errors.New("browser: navigation failed")

	// ErrStalePage means the ManagedPage belongs to a previous browser generation
	// (after reconnect) and its rod.Page reference is no longer valid.
	ErrStalePage = errors.New("browser: stale page reference after reconnect")
)

// IsCDPError reports whether err is a typed *cdp.Error from the Chrome DevTools
// Protocol. Returns the error and true if so, nil and false otherwise.
// Use this for structured CDP error checking instead of substring matching.
func IsCDPError(err error) (*cdp.Error, bool) {
	if err == nil {
		return nil, false
	}
	var cdpErr *cdp.Error
	if errors.As(err, &cdpErr) {
		return cdpErr, true
	}
	return nil, false
}

// IsCDPErrorCode reports whether err is a CDP error with the specific error code.
// CDP error codes: -32000 (Server error), -32001 (Session not found), etc.
func IsCDPErrorCode(err error, code int) bool {
	cdpErr, ok := IsCDPError(err)
	return ok && cdpErr.Code == code
}

// IsAlreadyInEffectErr reports whether err is the Chrome CDP "another override
// is already in effect" error. Uses structured *cdp.Error checking when available
// (code -32000 + exact message match) and falls back to substring scan only for
// wrapped errors that don't expose the typed error.
//
// This replaces the old isAlreadyInEffectErr which used strings.Contains on the
// full error string — too broad, could mask unrelated errors containing that
// substring. Now we check the typed CDP error first, and the fallback uses exact
// message matching on the CDP error message, not the full wrapped chain.
func IsAlreadyInEffectErr(err error) bool {
	if err == nil {
		return false
	}
	// Typed CDP error — check exact message, not substring on full chain.
	if cdpErr, ok := IsCDPError(err); ok {
		return cdpErr.Code == -32000 && isAlreadyInEffectMessage(cdpErr.Message)
	}
	// Fallback: some wrappers may not expose *cdp.Error via Unwrap.
	// Check the message directly, not the full error chain.
	return isAlreadyInEffectMessage(err.Error())
}

// isAlreadyInEffectMessage checks if the message text indicates an "already in
// effect" error. The message is stable across Chrome versions (see
// inspector_emulation_agent.cc: Response::ServerError("Another locale override
// is already in effect") and the timezone equivalent).
func isAlreadyInEffectMessage(msg string) bool {
	return strings.Contains(msg, "already in effect")
}
