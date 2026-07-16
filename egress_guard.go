// Package browser — CDP-level SSRF egress guard.
//
// go-browser drives a shared, remote headless Chrome (CloakBrowser) on
// behalf of go-wowa's render, chrome_interact, screenshot_url, solve_cf, and
// security_scan(depth=full) tools. Chrome — not this Go process — performs
// the actual outbound TCP dial for a caller-supplied URL, so the go-kit/
// httputil SSRF guard (which wraps net/http clients) cannot intervene here:
// this process only issues CDP commands over a WebSocket to the remote
// Chrome process.
//
// installEgressGuard closes that gap via CDP's Fetch domain, for HTTP(S)
// resource loads specifically: every such request Chrome is about to make —
// the initial navigation, each redirect hop (per the CDP spec, "Requests
// resulting from a redirect will have redirectedRequestId field set" — i.e.
// a redirect target is reported as its own fresh Fetch.requestPaused event,
// paused BEFORE Chrome dials it), and every subresource (image/script/XHR/
// fetch) — is paused and checked against the shared go-kit SSRF blocklist
// (httputil.CheckRawURL) before Chrome is allowed to proceed. A blocked
// request fails with NetworkErrorReasonBlockedByClient instead of reaching
// an internal target.
//
// # Accepted residuals (not closed by this file)
//
// WebSocket/WebRTC: CDP's Fetch domain intercepts HTTP(S) resource loads
// only — it does NOT intercept ws://, wss://, or WebRTC (ICE/STUN/TURN)
// traffic, which Chrome negotiates through an entirely different network
// path. A page that opens a WebSocket or a WebRTC data channel to an
// internal target is NOT caught here. Closing that residual requires a
// Chrome-launch-layer control (e.g. --force-webrtc-ip-handling-policy, or
// routing all traffic through a filtering forward proxy) — go-browser
// never launches Chrome itself (NewChromeManager only CONNECTS to an
// already-running remote CloakBrowser over CDP; the launch flags live in
// that container's own init script, a different repo/deploy surface).
// Tracked as a follow-up there, not fixed in this file.
//
// DNS-rebind TOCTOU: httputil.CheckRawURL resolves the hostname in THIS
// process at check time; Chrome performs its OWN, independent DNS
// resolution when it actually dials. A hostname that resolves to a public
// IP during our check but to an internal IP by the time Chrome connects
// (a classic DNS-rebind) is not caught by this Request-stage check alone —
// CDP's Fetch domain does not expose the resolved remote address at the
// Request stage (only at Response stage, via a separately-enabled Network
// domain event, correlated by request ID, well after the connection is
// already made). This is the SAME accepted tradeoff go-kit's CheckURL
// itself documents for any delegate a process cannot dial-guard directly
// (see httputil/ssrf.go), and the same one the crypto-security review
// already accepted as mergeable for go-enriche's identical delegate-facing
// guard. A Response-stage recheck (abort before the page reads the body,
// once the resolved remoteIPAddress is known) would narrow this window
// further; deferred as a separate, dedicated change — it requires enabling
// a second CDP domain and correlating cross-domain request IDs, which is
// more surface than this file should carry alongside the redirect-hop fix.
package browser

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anatolykoptev/go-kit/httputil"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// checkTimeout bounds httputil.CheckRawURL's DNS resolution so a hanging or
// slow-to-respond authoritative server for a caller-supplied hostname stalls
// only that one paused request, not the browser connection.
const checkTimeout = 5 * time.Second

// allowLocalhost, set from the EGRESS_ALLOW_LOCALHOST env var, relaxes the
// SSRF egress guard for loopback and private addresses. This is a
// operator-controlled escape hatch for development/testing where Chrome
// (CloakBrowser) runs in a separate container from the target local server —
// the SSRF guard blocks localhost/127.0.0.1/RFC1918 by default because a
// caller-supplied URL reaching internal infrastructure is the classic SSRF
// vector, but a local dev server on the host or a sibling container is a
// legitimate target that the guard was never meant to stop.
//
// NEVER enable in production where chrome_interact/render/screenshot_url
// accept URLs from untrusted callers (LLM-generated, user-submitted) — a
// blocked localhost is the guard doing its job there. The flag is parsed once
// at init from EGRESS_ALLOW_LOCALHOST=true/1/yes.
var allowLocalhost = parseBoolEnv("EGRESS_ALLOW_LOCALHOST")

// #27: Warn at init if EGRESS_ALLOW_LOCALHOST is enabled — this is a security
// escape hatch that should NEVER be on in production where untrusted callers
// can supply URLs. The warning is logged once at package init.
func init() {
	if allowLocalhost {
		slog.Warn("egress guard: EGRESS_ALLOW_LOCALHOST is enabled — SSRF guard will NOT block localhost/private IPs. " +
			"This is a development escape hatch — NEVER enable in production with untrusted caller URLs.")
	}
}

func parseBoolEnv(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "true" || v == "1" || v == "yes"
}

// isLocalhostURL reports whether u's host resolves to a loopback or private
// address — the class the SSRF guard blocks but allowLocalhost exempts.
// Resolves the hostname (with a bounded timeout) to check all returned IPs;
// a literal IP is checked directly.
func isLocalhostURL(ctx context.Context, u *url.URL) bool {
	host := u.Hostname()
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return httputil.IsBlockedIP(ip)
	}
	// "localhost" and "*.localhost" are always loopback per RFC 6761.
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	resolver := net.DefaultResolver
	if r, ok := ctx.Value(resolverKey{}).(*net.Resolver); ok && r != nil {
		resolver = r
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, a := range addrs {
		if httputil.IsBlockedIP(a.IP) {
			return true
		}
	}
	return false
}

type resolverKey struct{}

// egressGuard is the single, connection-wide owner of the CDP Fetch domain
// for one Chrome connection (see installEgressGuard). It merges two
// concerns that MUST share one Fetch.enable/disable lifecycle, because
// Fetch is a per-CDP-session domain, not a per-BrowserContext one:
//
//  1. SSRF egress checking (respondPaused) — every FetchRequestPaused event,
//     regardless of which BrowserContext or page it came from, is checked
//     against the shared go-kit SSRF blocklist.
//  2. Upstream residential-proxy authentication (respondAuth) — answering
//     FetchAuthRequired challenges for a caller-supplied authenticated
//     proxy (`http://user:pass@host:port`), reachable TODAY via the
//     `proxy` field on go-wowa's render/chrome_interact/solve_cf/
//     screenshot_url/snapshot MCP tool inputs — independent of whether the
//     service-wide CHROME_PROXY_URL env var is configured.
//
// Before this type existed, (2) was a SEPARATE Fetch.enable/EachEvent/
// Fetch.disable cycle (the old chrome_proxy_auth.go:setupProxyAuth,
// removed), installed per-context/per-call and torn down via
// `defer cleanup()` at every NewContext / RunInteract call site. Two
// independent Fetch listeners on the SAME root CDP session raced on every
// paused request — whichever responded first won, and proxy-auth's naive
// immediate continue (no SSRF check at all) could beat the guard's
// DNS-bound check — and proxy-auth's own Fetch.disable on cleanup killed
// the WHOLE domain, including the guard, for the REST of the connection
// (every later render/chrome_interact/solve_cf call, not just the one that
// used a proxy), until the next reconnect. Folding both into one listener
// removes both defects structurally: there is exactly one continue/fail
// decision per FetchRequestPaused event (the guard's), and nothing calls
// Fetch.disable except at connection teardown — installEgressGuard's own
// owner never does, the domain lives for the connection's lifetime.
//
// Known accepted limitation: registerProxyAuth holds ONE active credential
// set for the whole connection. This matches the pre-existing behavior it
// replaces — the old setupProxyAuth was ALSO root-session-scoped (proxy
// requests already routed through the shared root browser object, not a
// genuinely isolated per-context CDP session), so two concurrent contexts
// with DIFFERENT proxy credentials were already unsupported before this
// change, not a regression introduced here. A generation token in
// registerProxyAuth prevents a STALE unregister from clobbering a NEWER,
// still-active registration (see its doc comment) — a real safety
// improvement over the prior code, though it does not make concurrent
// different-credential proxies work correctly. That remains a known gap;
// closing it fully would need per-context/per-frame credential routing,
// out of scope for this SSRF-focused change.
type egressGuard struct {
	mu       sync.Mutex
	gen      uint64
	username string
	password string
	active   bool

	// #15: rebindDomains tracks hostnames whose response-stage remoteIPAddress
	// was a blocked IP (DNS rebind detected). Future Request-stage checks for
	// these domains will be blocked regardless of current DNS resolution.
	// Protected by mu.
	rebindDomains map[string]bool
}

// installEgressGuard enables CDP Fetch-domain interception on b — exactly
// once per Chrome connection (see chrome.go: NewChromeManager and
// chrome_lifecycle.go: reconnect), covering every BrowserContext and page
// created under it for the connection's lifetime — and returns the guard so
// NewContext / RunInteract can register upstream-proxy-auth credentials on
// it (see registerProxyAuth). Never call this more than once per live
// connection; see the egressGuard doc comment for why a second, independent
// Fetch listener is unsafe.
//
// Returns an error if the Fetch domain cannot be enabled — callers MUST
// treat that as fatal (refuse the connection) rather than proceed
// unguarded, since Chrome would otherwise dial every request unchecked.
func installEgressGuard(b *rod.Browser) (*egressGuard, error) {
	if err := (proto.FetchEnable{HandleAuthRequests: true}).Call(b); err != nil {
		return nil, fmt.Errorf("browser: enable Fetch domain for egress guard: %w", err)
	}

	// #15: Enable Network domain to capture response-stage remoteIPAddress.
	// This allows detecting DNS-rebinding attacks: the Request-stage check
	// resolves the hostname in THIS process, but Chrome does its own DNS
	// resolution. If the IP changed between check and dial (DNS rebind), the
	// response's remoteIPAddress will differ from what we checked.
	// We can't prevent the connection (it's already made), but we detect and
	// alert on the rebind, and track the domain for future Request-stage checks.
	if err := (proto.NetworkEnable{}).Call(b); err != nil {
		slog.Warn("egress guard: failed to enable Network domain for DNS-rebind detection — residual risk", "err", err)
	}

	g := &egressGuard{}

	wait := b.EachEvent(
		func(ev *proto.FetchRequestPaused) {
			go g.respondPaused(b, ev)
		},
		func(ev *proto.FetchAuthRequired) {
			go g.respondAuth(b, ev)
		},
		func(ev *proto.NetworkResponseReceived) {
			g.checkResponseIP(b, ev)
		},
	)
	go wait()

	return g, nil
}

// respondPaused checks one paused request's target against the shared
// go-kit SSRF blocklist and either continues or fails it. Runs in its own
// goroutine per event (bounded by checkTimeout) so one slow DNS lookup
// never blocks the EachEvent dispatch loop for other concurrent requests.
// This is the ONLY handler that ever responds to a FetchRequestPaused event
// on this connection — see the egressGuard doc comment for why a second,
// independent handler (the old proxy-auth listener) was a race.
func (g *egressGuard) respondPaused(b *rod.Browser, ev *proto.FetchRequestPaused) {
	if ev.Request == nil {
		// No request details to check — fail closed rather than guess.
		if err := (proto.FetchFailRequest{
			RequestID:   ev.RequestID,
			ErrorReason: proto.NetworkErrorReasonBlockedByClient,
		}).Call(b); err != nil {
			slog.Error("egress guard: fail-closed response (nil request) itself failed — target may still be reachable", "err", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	// Operator-controlled localhost bypass: when EGRESS_ALLOW_LOCALHOST is
	// set, skip the SSRF check for URLs that resolve to loopback/private
	// addresses. The scheme allowlist (http/https only) is still enforced
	// by CheckRawURL below for non-local URLs — the bypass is narrowly
	// scoped to the address class the guard blocks, not a blanket skip.
	if allowLocalhost {
		if parsed, perr := url.Parse(ev.Request.URL); perr == nil && isLocalhostURL(ctx, parsed) {
			if err := (proto.FetchContinueRequest{RequestID: ev.RequestID}).Call(b); err != nil {
				slog.Warn("egress guard: continue request (localhost bypass) failed", "url", ev.Request.URL, "err", err)
			}
			return
		}
	}

	// #15: Check if this domain was previously flagged for DNS rebinding.
	// If so, block immediately regardless of current DNS resolution.
	if parsed, perr := url.Parse(ev.Request.URL); perr == nil && parsed.Hostname() != "" {
		if g.isRebindDomain(parsed.Hostname()) {
			slog.Warn("egress guard: blocking request to DNS-rebind-flagged domain",
				"url", ev.Request.URL, "host", parsed.Hostname())
			if failErr := (proto.FetchFailRequest{
				RequestID:   ev.RequestID,
				ErrorReason: proto.NetworkErrorReasonBlockedByClient,
			}).Call(b); failErr != nil {
				slog.Error("egress guard: FAILED to block rebind-flagged request", "url", ev.Request.URL, "err", failErr)
			}
			return
		}
	}

	if err := httputil.CheckRawURL(ctx, ev.Request.URL); err != nil {
		// err's text already embeds the resolved blocked IP (see
		// httputil.CheckURL) — nothing more to extract for the log line.
		slog.Warn("egress guard: blocked request",
			"url", ev.Request.URL,
			"resource_type", ev.ResourceType,
			"err", err,
		)
		if failErr := (proto.FetchFailRequest{
			RequestID:   ev.RequestID,
			ErrorReason: proto.NetworkErrorReasonBlockedByClient,
		}).Call(b); failErr != nil {
			// A lost block must never vanish silently: this is the one CDP
			// call in this file whose failure means Chrome may still reach
			// the blocked target unobstructed.
			slog.Error("egress guard: FAILED to block request — target may still be reachable",
				"url", ev.Request.URL, "err", failErr)
		}
		return
	}

	if err := (proto.FetchContinueRequest{RequestID: ev.RequestID}).Call(b); err != nil {
		slog.Warn("egress guard: continue request failed", "url", ev.Request.URL, "err", err)
	}
}

// checkResponseIP verifies the remoteIPAddress from a Network.responseReceived
// event against the SSRF blocklist. This is the Response-stage DNS-rebind
// detection (#15): if the IP Chrome actually connected to is blocked, but the
// Request-stage URL check passed (because DNS resolved differently at check
// time), we log the rebind and track the domain for future blocking.
//
// We cannot abort the connection at this point (it's already established), but
// we CAN:
//  1. Alert at Error level so operators see the DNS-rebind attempt
//  2. Add the domain to rebindDomains so future Request-stage checks block it
//     immediately, regardless of what DNS resolves to
func (g *egressGuard) checkResponseIP(b *rod.Browser, ev *proto.NetworkResponseReceived) {
	if ev.Response == nil || ev.Response.RemoteIPAddress == "" {
		return
	}
	ip := net.ParseIP(ev.Response.RemoteIPAddress)
	if ip == nil {
		return
	}
	if !httputil.IsBlockedIP(ip) {
		return
	}
	// DNS rebind detected: Chrome connected to a blocked IP that our
	// Request-stage check didn't catch (hostname resolved differently).
	host := ""
	if ev.Response.URL != "" {
		if u, err := url.Parse(ev.Response.URL); err == nil {
			host = u.Hostname()
		}
	}
	if allowLocalhost && host != "" {
		// If localhost is allowed and the IP is loopback, don't flag it.
		if ip.IsLoopback() {
			return
		}
	}
	slog.Error("egress guard: DNS REBIND DETECTED — Chrome connected to blocked IP despite Request-stage check passing",
		"url", ev.Response.URL,
		"remote_ip", ev.Response.RemoteIPAddress,
		"host", host,
		"request_id", ev.RequestID,
	)
	// Track the domain so future requests are blocked at Request stage.
	if host != "" {
		g.mu.Lock()
		if g.rebindDomains == nil {
			g.rebindDomains = make(map[string]bool)
		}
		g.rebindDomains[host] = true
		g.mu.Unlock()
	}
}

// isRebindDomain checks if a hostname has been flagged for DNS rebinding.
func (g *egressGuard) isRebindDomain(host string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rebindDomains[host]
}

// respondAuth answers an upstream-proxy authentication challenge using the
// currently registered credentials (see registerProxyAuth), or cancels the
// challenge if none are registered — never falls through to Chrome's
// default behavior (which could otherwise hang headless waiting on its own
// credentials UI).
func (g *egressGuard) respondAuth(b *rod.Browser, ev *proto.FetchAuthRequired) {
	g.mu.Lock()
	active, username, password := g.active, g.username, g.password
	g.mu.Unlock()

	req := proto.FetchContinueWithAuth{RequestID: ev.RequestID}
	if active {
		req.AuthChallengeResponse = &proto.FetchAuthChallengeResponse{
			Response: proto.FetchAuthChallengeResponseResponseProvideCredentials,
			Username: username,
			Password: password,
		}
	} else {
		req.AuthChallengeResponse = &proto.FetchAuthChallengeResponse{
			Response: proto.FetchAuthChallengeResponseResponseCancelAuth,
		}
	}
	if err := req.Call(b); err != nil {
		slog.Warn("egress guard: auth challenge response failed", "err", err)
	}
}

// registerProxyAuth activates credentials for upstream-proxy authentication
// on THIS connection's shared Fetch domain (see the egressGuard doc comment
// for why there is only one active slot). Returns an unregister func the
// caller MUST invoke once its context/page no longer needs the proxy
// (mirrors the old setupProxyAuth cleanup contract) — unlike that old
// contract, this NEVER disables the Fetch domain, only clears the
// credential slot, and only if a newer registration hasn't already replaced
// it: a generation token guards against a stale unregister (from an earlier,
// already-finished call) wiping out a still-active, more recent
// registration from a different concurrent call.
func (g *egressGuard) registerProxyAuth(username, password string) func() {
	g.mu.Lock()
	g.gen++
	myGen := g.gen
	g.username = username
	g.password = password
	g.active = true
	g.mu.Unlock()

	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.gen != myGen {
			// A newer registration already took the slot — don't clear
			// its credentials out from under it.
			return
		}
		g.username = ""
		g.password = ""
		g.active = false
	}
}
