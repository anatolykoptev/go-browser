package browser

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/httputil"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// navigateTimeout bounds how long a test waits for Page.Navigate to resolve
// (it blocks until the top-level frame commits or definitively fails — see
// go-rod's Page.Navigate, which surfaces a blocked/failed navigation via
// res.ErrorText, not just a hang).
const navigateTimeout = 8 * time.Second

// firstNonBlockedIP returns a local address (IPv4 or IPv6) this box owns
// that httputil.IsBlockedIP does NOT flag -- i.e. one Chrome will treat as a
// legitimate "public" target when a test binds an httptest.Server to it.
// This is what makes the redirect-hop test meaningful: hop 1 must look
// allowed for hop 2's block to prove the guard re-checks EACH hop rather
// than only the URL originally handed to Page.Navigate. Cloud VMs (this repo
// runs its self-hosted CI on one) commonly expose their public address only
// via IPv6 on the local interface -- the public IPv4 is handled by the
// provider's NAT/floating-IP layer and never appears in `ip addr` -- so both
// families are checked here, IPv4 preferred. Returns ok=false if this box
// has no such address (e.g. a fully NAT'd sandbox with only private ranges),
// in which case the calling test should skip rather than false-fail.
func firstNonBlockedIP(t *testing.T) (string, bool) {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", false
	}
	var v6Fallback string
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || httputil.IsBlockedIP(ipNet.IP) {
			continue
		}
		if ipNet.IP.To4() != nil {
			return ipNet.IP.String(), true
		}
		if v6Fallback == "" {
			v6Fallback = ipNet.IP.String()
		}
	}
	if v6Fallback != "" {
		return v6Fallback, true
	}
	return "", false
}

// listenOn binds an httptest-compatible listener to ip, skipping the test if
// the box can't bind that address (e.g. it's assigned to a different netns).
func listenOn(t *testing.T, ip string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Skipf("cannot bind %s: %v", ip, err)
	}
	return ln
}

// sharedGuard mirrors acquireSharedBrowser's sync.Once pattern: installing
// the egress guard is meant to happen exactly ONCE per Chrome connection
// (see installEgressGuard's doc comment — a second independent install
// would itself reintroduce the double-listener race this file exists to
// prevent), so every test in this file installs onto the SAME shared test
// browser exactly once and reuses the resulting *egressGuard handle.
var (
	sharedGuardOnce     sync.Once
	sharedGuardInstance *egressGuard
	sharedGuardErr      error
)

// acquireGuard installs the egress guard on the shared test browser (once)
// and returns the guard handle, so a test can also drive registerProxyAuth
// directly (see TestEgressGuard_CoexistsWithActiveProxyAuth_StillBlocks).
func acquireGuard(t *testing.T, b *rod.Browser) *egressGuard {
	t.Helper()
	sharedGuardOnce.Do(func() {
		sharedGuardInstance, sharedGuardErr = installEgressGuard(b)
	})
	if sharedGuardErr != nil {
		t.Fatalf("installEgressGuard: %v", sharedGuardErr)
	}
	return sharedGuardInstance
}

// newGuardedPage installs the egress guard on b (the same install path
// NewChromeManager/reconnect use — see egress_guard.go) and returns a fresh
// blank page for the test to navigate.
func newGuardedPage(t *testing.T, b *rod.Browser) *rod.Page {
	t.Helper()
	acquireGuard(t, b)
	page, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	t.Cleanup(func() { _ = page.Close() })
	return page
}

// TestEgressGuard_DirectInternalTarget_Blocked proves the guard refuses a
// navigation whose target is itself internal (the baseline case: no redirect
// involved). httptest.Server binds 127.0.0.1, which is exactly the kind of
// target this guard exists to keep Chrome from dialing.
func TestEgressGuard_DirectInternalTarget_Blocked(t *testing.T) {
	b := acquireSharedBrowser(t)
	page := newGuardedPage(t, b)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("should never be served"))
	}))
	defer srv.Close()

	err := page.Timeout(navigateTimeout).Navigate(srv.URL)
	if err == nil {
		t.Fatalf("navigate to internal target %q: want error, got nil", srv.URL)
	}
}

// TestEgressGuard_PublicTarget_Allowed is the exit-criteria smoke test: the
// guard must not refuse a legitimate, non-internal target. Without this,
// TestEgressGuard_RedirectToInternalTarget_BlockedAtHop would prove nothing —
// if the guard blocked everything, the redirect would never even reach hop 2.
func TestEgressGuard_PublicTarget_Allowed(t *testing.T) {
	b := acquireSharedBrowser(t)
	page := newGuardedPage(t, b)

	ip, ok := firstNonBlockedIP(t)
	if !ok {
		t.Skip("no non-blocked local interface IP available in this environment")
	}

	const body = "hello from a public-looking origin"
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	srv.Listener = listenOn(t, ip)
	srv.Start()
	defer srv.Close()

	if err := page.Timeout(navigateTimeout).Navigate(srv.URL); err != nil {
		t.Fatalf("navigate to allowed target %q: %v", srv.URL, err)
	}
	_ = page.WaitLoad()
	html, err := page.HTML()
	if err != nil {
		t.Fatalf("page.HTML: %v", err)
	}
	if !strings.Contains(html, body) {
		t.Errorf("page did not load expected content; got: %s", html)
	}
}

// TestEgressGuard_RedirectToInternalTarget_BlockedAtHop is the headline P0b
// case: a target that LOOKS public at Page.Navigate time (hop 1, allowed)
// 302-redirects to an internal target (hop 2). A guard that only checked the
// URL string handed to Navigate once, up front, would miss this entirely —
// proving the fix requires the CDP Fetch-domain per-hop recheck this file
// implements, not a pre-navigate string check.
func TestEgressGuard_RedirectToInternalTarget_BlockedAtHop(t *testing.T) {
	b := acquireSharedBrowser(t)
	page := newGuardedPage(t, b)

	ip, ok := firstNonBlockedIP(t)
	if !ok {
		t.Skip("no non-blocked local interface IP available in this environment")
	}

	reached := make(chan struct{}, 1)
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case reached <- struct{}{}:
		default:
		}
		_, _ = w.Write([]byte("should never be served"))
	}))
	defer internal.Close()

	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	origin.Listener = listenOn(t, ip)
	origin.Start()
	defer origin.Close()

	err := page.Timeout(navigateTimeout).Navigate(origin.URL)
	if err == nil {
		t.Fatalf("navigate through redirect to internal target %q: want error, got nil", internal.URL)
	}

	select {
	case <-reached:
		t.Fatalf("internal redirect target %q was reached — guard did not re-check the redirect hop", internal.URL)
	default:
	}
}

// TestEgressGuard_MetadataAddress_Blocked pins the cloud-metadata address
// specifically, since it's the concrete SSRF payload this whole arc exists
// to close (a redirect to 169.254.169.254 needs no DNS-rebind timing race —
// see the plan's crypto-security re-review finding).
func TestEgressGuard_MetadataAddress_Blocked(t *testing.T) {
	b := acquireSharedBrowser(t)
	page := newGuardedPage(t, b)

	// 169.254.169.254 with an unroutable port: the guard must fail the
	// request before any connection is attempted, so which port is used
	// doesn't matter — if the guard were absent, Chrome would still try
	// (and fail to connect, but for the WRONG reason: connection refused
	// rather than blocked-by-client), so we assert on the specific error.
	err := page.Timeout(navigateTimeout).Navigate("http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatalf("navigate to cloud-metadata address: want error, got nil")
	}
	if !strings.Contains(err.Error(), "BLOCKED_BY_CLIENT") {
		t.Errorf("navigate error = %q, want it to mention BLOCKED_BY_CLIENT (guard-originated, not a connect failure)", err.Error())
	}
}

// TestEgressGuard_CoexistsWithActiveProxyAuth_StillBlocks is the missing
// coexistence test the crypto-security review flagged: prove that a page
// created while upstream-proxy-auth credentials are ACTIVE on the shared
// egress guard (registerProxyAuth — what NewContext/RunInteract call for an
// authenticated `http://user:pass@host:port` proxy, reachable today via the
// `proxy` field on render/chrome_interact/solve_cf/screenshot_url/snapshot)
// is STILL refused for an internal target. Before this file's fix, proxy
// auth was a SEPARATE Fetch listener that (a) answered FetchRequestPaused
// immediately with no SSRF check at all, racing the guard's check, and (b)
// disabled the whole Fetch domain on cleanup, killing the guard
// connection-wide. Both defects are now structurally impossible — there is
// exactly one Fetch.enable and one FetchRequestPaused handler for the whole
// connection (see egress_guard.go) — this test exercises that end to end
// rather than trusting the architecture argument alone.
func TestEgressGuard_CoexistsWithActiveProxyAuth_StillBlocks(t *testing.T) {
	b := acquireSharedBrowser(t)
	guard := acquireGuard(t, b)

	// Simulate what NewContext/RunInteract do for an authenticated-proxy
	// call: activate credentials on the shared guard. No real upstream
	// proxy is needed for this assertion — it targets respondPaused's
	// gating behavior while active/username/password are non-zero, not the
	// FetchAuthRequired challenge-response flow itself (a separate concern,
	// unaffected by this test).
	unregister := guard.registerProxyAuth("coexist-test-user", "coexist-test-pass")
	defer unregister()

	page, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	defer func() { _ = page.Close() }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("should never be served"))
	}))
	defer srv.Close()

	navErr := page.Timeout(navigateTimeout).Navigate(srv.URL)
	if navErr == nil {
		t.Fatalf("navigate to internal target %q while proxy-auth active: want error, got nil", srv.URL)
	}
	if !strings.Contains(navErr.Error(), "BLOCKED_BY_CLIENT") {
		t.Errorf("navigate error = %q, want it to mention BLOCKED_BY_CLIENT (guard-originated, not some other failure)", navErr.Error())
	}
}
