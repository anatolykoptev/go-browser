package browser

import (
	"fmt"
	"log/slog"
	"net/url"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// NewContext creates an isolated BrowserContext with optional proxy.
// Returns a browser scoped to that context, the context ID for lifecycle management,
// and a cleanup function for proxy auth handling.
// Supports authenticated proxies (http://user:pass@host:port) via CDP Fetch.authRequired.
func (m *ChromeManager) NewContext(proxy string) (*rod.Browser, proto.BrowserBrowserContextID, func(), error) {
	b := m.getBrowser()
	if b == nil {
		return nil, "", nil, ErrUnavailable
	}

	proxyServer, proxyUser, proxyPass := parseProxy(proxy)

	createCtx := func(browser *rod.Browser) (*proto.TargetCreateBrowserContextResult, error) {
		return proto.TargetCreateBrowserContext{
			ProxyServer:     proxyServer,
			DisposeOnDetach: true,
		}.Call(browser)
	}

	res, err := createCtx(b)
	if err != nil {
		if reconnErr := m.reconnect(); reconnErr != nil {
			return nil, "", nil, fmt.Errorf("chrome: create browser context: %w (reconnect failed: %v)", err, reconnErr)
		}
		b = m.getBrowser()
		if b == nil {
			return nil, "", nil, fmt.Errorf("chrome: reconnect succeeded but browser is nil")
		}
		res, err = createCtx(b)
		if err != nil {
			return nil, "", nil, fmt.Errorf("chrome: create browser context after reconnect: %w", err)
		}
	}

	scoped := b.NoDefaultDevice()
	scoped.BrowserContextID = res.BrowserContextID

	// Register proxy-auth credentials on the connection-wide egress guard
	// (see egress_guard.go) if provided — NEVER a separate Fetch.enable/
	// disable cycle, which would race with and could kill the SSRF guard.
	// Read the guard AFTER any reconnect above, since reconnect installs a
	// fresh one on the new connection.
	var cleanup func()
	if proxyUser != "" {
		guard := m.getGuard()
		if guard != nil {
			cleanup = guard.registerProxyAuth(proxyUser, proxyPass)
		} else {
			// #21: Log when proxy auth is silently skipped — the egress guard is nil
			// (e.g., during reconnect or if installEgressGuard failed). Without this,
			// authenticated proxy requests will fail with 407 Proxy Authentication
			// Required and the caller has no idea why.
			slog.Warn("chrome: proxy auth registration skipped — egress guard is nil (reconnect in progress?)", "proxyUser", proxyUser)
		}
	}

	return scoped, res.BrowserContextID, cleanup, nil
}

// parseProxy extracts host:port and credentials from a proxy URL.
// Input:  "http://user:pass@host:port" → ("host:port", "user", "pass")
// Input:  "http://host:port"           → ("http://host:port", "", "")
// Input:  ""                           → ("", "", "")
func parseProxy(raw string) (server, user, pass string) {
	if raw == "" {
		return "", "", ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw, "", ""
	}
	pass, _ = u.User.Password()
	user = u.User.Username()
	// Reconstruct URL without credentials for Chrome's ProxyServer.
	u.User = nil
	return u.String(), user, pass
}
