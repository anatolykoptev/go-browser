package browser

import "errors"

var (
	// ErrUnavailable means the backend is not connected or binary not found.
	ErrUnavailable = errors.New("browser: backend unavailable")

	// ErrTimeout means the render exceeded the configured deadline.
	ErrTimeout = errors.New("browser: render timeout")

	// ErrNavigate means page navigation failed (DNS, TLS, HTTP error).
	ErrNavigate = errors.New("browser: navigation failed")
)
