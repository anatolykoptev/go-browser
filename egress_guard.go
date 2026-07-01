// Package browser — CDP-level SSRF egress guard.
//
// go-browser drives a shared, remote headless Chrome (CloakBrowser) that
// navigates to caller-supplied URLs on behalf of go-wowa's render,
// chrome_interact, screenshot_url, solve_cf, and security_scan(depth=full)
// tools. Chrome — not this Go process — performs the actual outbound TCP
// dial, so a Go-level net/http SSRF guard (as go-kit/httputil provides for a
// process that dials for itself) cannot intervene here: this process only
// issues CDP commands over a WebSocket to the remote Chrome process.
//
// installEgressGuard closes that gap via CDP's Fetch domain: every request
// Chrome is about to make — the initial navigation, EACH redirect hop (per
// the CDP spec, "Requests resulting from a redirect will have
// redirectedRequestId field set" — i.e. a redirect target is reported as its
// own fresh Fetch.requestPaused event, paused BEFORE Chrome dials it), and
// every subresource (image/script/XHR/fetch) — is paused and checked against
// the shared go-kit SSRF blocklist (httputil.CheckRawURL) before Chrome is
// allowed to proceed. A blocked request fails with
// NetworkErrorReasonBlockedByClient instead of reaching an internal target.
package browser

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/anatolykoptev/go-kit/httputil"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// installEgressGuard enables CDP Fetch-domain interception on b and checks
// every paused request's URL via httputil.CheckRawURL. b must be the ONE
// long-lived root *rod.Browser connection for a ChromeManager (see chrome.go
// and chrome_lifecycle.go): Fetch.enable issued at that scope covers every
// BrowserContext and page created under it for the lifetime of the
// connection — the default-mode shared context AND proxy/private-mode
// isolated contexts alike (github.com/anatolykoptev/go-wowa's
// internal/chrome.ContextPool) — so this is called exactly once per Chrome
// connection, at initial connect (chrome.go: NewChromeManager) and again
// after every reconnect (chrome_lifecycle.go: reconnect), never per-request
// or per-context.
//
// Coexistence with per-context proxy auth (chrome_proxy_auth.go): that code
// installs its OWN Fetch.enable + FetchRequestPaused listener scoped to one
// freshly-created BrowserContext, purely to unblock non-auth requests while
// answering FetchAuthRequired challenges for an upstream residential proxy.
// Both listeners may independently receive and respond to the same
// FetchRequestPaused event when a proxy is configured; whichever responds
// first wins, and the second response is a harmless discarded error from
// Chrome's perspective (the request is already resolved — every call here
// discards its error the same way chrome_proxy_auth.go already does).
// FetchAuthRequired is a distinct CDP event this guard never subscribes to,
// so proxy authentication itself is untouched.
//
// Returns an error if the Fetch domain cannot be enabled — callers MUST
// treat that as fatal (refuse the connection) rather than proceed
// unguarded, since Chrome would otherwise dial every request unchecked.
func installEgressGuard(b *rod.Browser) error {
	if err := (proto.FetchEnable{}).Call(b); err != nil {
		return fmt.Errorf("browser: enable Fetch domain for egress guard: %w", err)
	}

	wait := b.EachEvent(func(ev *proto.FetchRequestPaused) {
		go respondGuarded(b, ev)
	})
	go wait()

	return nil
}

// respondGuarded checks one paused request's target against the shared
// go-kit SSRF blocklist and either continues or fails it. Runs in its own
// goroutine per event (matching the existing chrome_proxy_auth.go pattern)
// so one slow DNS lookup never blocks the EachEvent dispatch loop for other
// concurrent requests.
func respondGuarded(b *rod.Browser, ev *proto.FetchRequestPaused) {
	if ev.Request == nil {
		// No request details to check — fail closed rather than guess.
		_ = proto.FetchFailRequest{
			RequestID:   ev.RequestID,
			ErrorReason: proto.NetworkErrorReasonBlockedByClient,
		}.Call(b)
		return
	}

	if err := httputil.CheckRawURL(context.Background(), ev.Request.URL); err != nil {
		// err's text already embeds the resolved blocked IP (see
		// httputil.CheckURL) — nothing more to extract for the log line.
		slog.Warn("egress guard: blocked request",
			"url", ev.Request.URL,
			"resource_type", ev.ResourceType,
			"err", err,
		)
		_ = proto.FetchFailRequest{
			RequestID:   ev.RequestID,
			ErrorReason: proto.NetworkErrorReasonBlockedByClient,
		}.Call(b)
		return
	}

	_ = proto.FetchContinueRequest{RequestID: ev.RequestID}.Call(b)
}
