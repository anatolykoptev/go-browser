package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

// TestContextPool_NamedSession_EmptyMode_LandsInPersistentContext verifies
// rule 1 (#74): a named session with an empty mode defaults to the persistent
// ("default") context, NOT an ephemeral incognito jar.
//
// The test sets a cookie in the default context first, then creates a named
// session with mode="" and asserts the session sees that cookie. Under the
// old behaviour (contextKey default-arm mapping "" → "private"), the named
// session would land in a fresh incognito context with an empty cookie jar.
//
// Mutation probe: remove the rule-1 line in GetOrCreatePage (session != "" &&
// mode == "" → mode = "default") → mode stays "" → contextKey maps "" to
// "private" → named session in incognito → does NOT see the cookie → RED.
func TestContextPool_NamedSession_EmptyMode_LandsInPersistentContext(t *testing.T) {
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	// 1. Seed a cookie in the default context via a default-context page.
	seed, err := p.GetOrCreatePage("seed-cookie", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("seed default page: %v", err)
	}
	defer func() { _ = p.ClosePage("seed-cookie") }()

	if err := seed.Page.SetCookies([]*proto.NetworkCookieParam{{
		Name:  "rule1-marker",
		Value: "persistent",
		URL:   "http://example.com/",
	}}); err != nil {
		t.Fatalf("set cookie: %v", err)
	}

	// 2. Create a named session with mode="" — rule 1 must resolve to "default".
	mp, err := p.GetOrCreatePage("named-sess-no-mode", "", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage(named, empty mode): %v", err)
	}
	defer func() { _ = p.ClosePage("named-sess-no-mode") }()

	// 3. Assert the named session sees the cookie → it's in the default context.
	cookies, err := mp.Page.Cookies([]string{"http://example.com/"})
	if err != nil {
		t.Fatalf("read cookies: %v", err)
	}
	found := false
	for _, c := range cookies {
		if c.Name == "rule1-marker" && c.Value == "persistent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("named session with empty mode did NOT see the default-context "+
			"cookie — it landed in an ephemeral jar (rule 1 not applied). "+
			"cookies seen: %d", len(cookies))
	}

	// 4. Assert the resolved mode is "default" (rule 3).
	if mp.Mode != "default" {
		t.Errorf("ManagedPage.Mode = %q, want %q (rule 1 resolution)", mp.Mode, "default")
	}
}

// TestContextPool_UnrecognisedMode_ReturnsError verifies rule 2 (#74): an
// unrecognised mode (e.g. "defualt") returns a typed error and creates NO
// context. The old contextKey default-arm silently absorbed typos into an
// incognito context.
//
// Mutation probe: restore the old default-arm (return "private", nil) → no
// error → a context is created → RED.
func TestContextPool_UnrecognisedMode_ReturnsError(t *testing.T) {
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	_, err := p.GetOrCreatePage("typo-session", "defualt", "", "about:blank")
	if err == nil {
		t.Fatal("GetOrCreatePage with typo mode returned nil error — typos must be rejected (rule 2)")
	}
	if !errors.Is(err, ErrInvalidMode) {
		t.Errorf("error is not ErrInvalidMode: got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "defualt") {
		t.Errorf("error does not name the offending value: %v", err)
	}
	if !strings.Contains(err.Error(), "default") || !strings.Contains(err.Error(), "private") || !strings.Contains(err.Error(), "proxy") {
		t.Errorf("error does not list the accepted set: %v", err)
	}

	// No context should have been created.
	p.contextsMu.RLock()
	n := len(p.contexts)
	p.contextsMu.RUnlock()
	if n != 0 {
		t.Errorf("contexts created despite error: %d entries (rule 2 must fail closed)", n)
	}
}

// TestContextPool_PrivateMode_StillIsolated verifies the regression guard for
// rule 1: mode="private" passed explicitly must still yield an isolated cookie
// jar that does NOT see the default context's cookies. If rule 1 were
// implemented too broadly (e.g. mapping ALL empty/unknown modes to "default"),
// this test would fail.
//
// Mutation probe: map "private" to "default" in contextKey → private session
// sees the cookie → RED.
func TestContextPool_PrivateMode_StillIsolated(t *testing.T) {
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	// 1. Seed a cookie in the default context.
	seed, err := p.GetOrCreatePage("seed-cookie-priv", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("seed default page: %v", err)
	}
	defer func() { _ = p.ClosePage("seed-cookie-priv") }()

	if err := seed.Page.SetCookies([]*proto.NetworkCookieParam{{
		Name:  "priv-marker",
		Value: "default-only",
		URL:   "http://example.com/",
	}}); err != nil {
		t.Fatalf("set cookie: %v", err)
	}

	// 2. Create a private session — must NOT see the default cookie.
	mp, err := p.GetOrCreatePage("priv-sess", "private", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage(private): %v", err)
	}
	defer func() { _ = p.ClosePage("priv-sess") }()

	cookies, err := mp.Page.Cookies([]string{"http://example.com/"})
	if err != nil {
		t.Fatalf("read cookies: %v", err)
	}
	for _, c := range cookies {
		if c.Name == "priv-marker" {
			t.Errorf("private session saw the default-context cookie %q — "+
				"isolation broken (rule 1 too broad)", c.Name)
		}
	}

	if mp.Mode != "private" {
		t.Errorf("ManagedPage.Mode = %q, want %q", mp.Mode, "private")
	}
}

// TestContextPool_ResolvedMode_Readable verifies rule 3 (#74): the resolved
// mode is readable from ManagedPage.Mode for each of the three accepted modes.
// A consumer reads mp.Mode to learn which context was actually used.
func TestContextPool_ResolvedMode_Readable(t *testing.T) {
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	cases := []struct {
		name  string
		mode  string
		proxy string // mode "proxy" requires one (ErrProxyRequired); never dialed at context creation
		want  string
	}{
		{"default", "default", "", "default"},
		{"private", "private", "", "private"},
		{"proxy", "proxy", "http://203.0.113.1:9", "proxy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := "mode-read-" + tc.name
			mp, err := p.GetOrCreatePage(sess, tc.mode, tc.proxy, "about:blank")
			if err != nil {
				t.Fatalf("GetOrCreatePage(%q): %v", tc.mode, err)
			}
			defer func() { _ = p.ClosePage(sess) }()

			if mp.Mode != tc.want {
				t.Errorf("ManagedPage.Mode = %q, want %q", mp.Mode, tc.want)
			}
		})
	}
}

// TestContextPool_NamedSession_EmptyMode_ResolvesToDefault_ResolveParams
// verifies rule 1 at the resolveSessionParams layer: a named session with no
// mode resolves to "default" (not "private" as before #74).
//
// Mutation probe: revert resolveSessionParams to mode = modePrivate → mode =
// "private" → RED.
func TestContextPool_NamedSession_EmptyMode_ResolvesToDefault_ResolveParams(t *testing.T) {
	_, mode, _, eph := resolveSessionParams(InteractRequest{Session: "my-session"})
	if mode != "default" {
		t.Errorf("resolveSessionParams named session + empty mode: mode=%q, want %q", mode, "default")
	}
	if eph {
		t.Errorf("named session should be persistent, got ephemeral=true")
	}
}

// TestContextPool_NamedSession_EmptyMode_WithProxy_ResolvesToProxy verifies
// rule 1 with a proxy: a named session + empty mode + proxy → "proxy" (the
// persistent context through that proxy).
func TestContextPool_NamedSession_EmptyMode_WithProxy_ResolvesToProxy(t *testing.T) {
	proxy := "http://user:pass@host:8080"
	_, mode, _, eph := resolveSessionParams(InteractRequest{Session: "my-session", Proxy: &proxy})
	if mode != "proxy" {
		t.Errorf("resolveSessionParams named session + empty mode + proxy: mode=%q, want %q", mode, "proxy")
	}
	if eph {
		t.Errorf("named session should be persistent, got ephemeral=true")
	}
}

// TestContextKey_RejectsTypos is a pure unit test for contextKey's validation
// (rule 2). No browser needed.
func TestContextKey_RejectsTypos(t *testing.T) {
	bad := []string{"defualt", "DEFAULT", "incognito", "persistent", " "}
	for _, m := range bad {
		t.Run(m, func(t *testing.T) {
			_, err := contextKey(m, "")
			if err == nil {
				t.Errorf("contextKey(%q) returned nil error — typos must be rejected", m)
			}
			if !errors.Is(err, ErrInvalidMode) {
				t.Errorf("contextKey(%q) error is not ErrInvalidMode: %v", m, err)
			}
		})
	}
}

// TestContextPool_NamedSession_EmptyMode_WithProxy_LandsInProxyContext
// verifies F3: GetOrCreatePage with a named session, empty mode AND a proxy
// must resolve to a PROXY context with proxyServer set — NOT drop the proxy
// and fall back to the unproxied default context (proxy bypass / datacenter-IP
// leak). resolveSessionParams already gets this right; GetOrCreatePage must
// agree.
//
// This test calls GetOrCreatePage directly (not resolveSessionParams) because
// the existing proxy test only exercises resolveSessionParams and does not
// cover the pool's rule-1 path.
//
// Mutation probe: restore the unconditional `mode = "default"` in
// GetOrCreatePage's rule 1 → contextKey("default", proxy) yields "default" →
// getOrCreateContextSafe's default branch never sets proxyServer → the session
// lands in an unproxied default context → mc.Mode != "proxy" → RED.
func TestContextPool_NamedSession_EmptyMode_WithProxy_LandsInProxyContext(t *testing.T) {
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	// Use a TEST-NET proxy address with credentials so parseProxy returns a
	// sanitized server. about:blank navigation never touches the proxy, so
	// TargetCreateBrowserContext succeeds without a live proxy.
	proxyRaw := "http://user:pass@192.0.2.1:9" // TEST-NET-1: public per the proxy guard, never routed
	mp, err := p.GetOrCreatePage("named-proxy-sess", "", proxyRaw, "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage(named, empty mode, proxy): %v", err)
	}
	defer func() { _ = p.ClosePage("named-proxy-sess") }()

	// 1. Resolved mode on the page must be "proxy" (rule 3).
	if mp.Mode != "proxy" {
		t.Fatalf("ManagedPage.Mode = %q, want %q (rule 1 with proxy must "+
			"resolve to proxy, not default — proxy bypass)", mp.Mode, "proxy")
	}

	// 2. The context that owns this session must be a proxy context with the
	// proxy recorded. If rule 1 dropped the proxy, the session would land in
	// the default context (Mode="default", Proxy="") and egress from the
	// datacenter IP.
	var owner *ManagedContext
	p.contextsMu.RLock()
	for _, mc := range p.contexts {
		mc.Mu.Lock()
		_, ok := mc.Pages["named-proxy-sess"]
		mc.Mu.Unlock()
		if ok {
			owner = mc
			break
		}
	}
	p.contextsMu.RUnlock()
	if owner == nil {
		t.Fatal("no context owns the named-proxy-sess session")
	}
	if owner.Mode != "proxy" {
		t.Errorf("owning context Mode = %q, want %q", owner.Mode, "proxy")
	}
	if owner.Proxy == "" {
		t.Errorf("owning context Proxy is empty — proxy was dropped (proxy bypass)")
	}
}

// TestContextKey_AcceptedModes is a pure unit test verifying the accepted mode
// set maps correctly. No browser needed.
func TestContextKey_AcceptedModes(t *testing.T) {
	cases := []struct {
		mode    string
		proxy   string
		wantKey string
	}{
		{"default", "", "default"},
		{"private", "", "private"},
		{"", "", "private"}, // anonymous ephemeral — out of scope for rule 1
		{"proxy", "http://p:80", "proxy:http://p:80"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			key, err := contextKey(tc.mode, tc.proxy)
			if err != nil {
				t.Fatalf("contextKey(%q, %q): %v", tc.mode, tc.proxy, err)
			}
			if key != tc.wantKey {
				t.Errorf("contextKey(%q, %q) = %q, want %q", tc.mode, tc.proxy, key, tc.wantKey)
			}
		})
	}
}

// TestContextKey_DefaultModeWithProxy_Rejected verifies #97 at the pure
// contextKey level: mode "default" with a non-empty proxy must return a typed
// error, while every other mode/proxy combination is unchanged. The default
// context has no proxy knob — honoring the request would silently drop the
// proxy and egress on the host's real IP.
//
// Mutation probe: remove the proxy != "" rejection in contextKey's "default"
// arm (context_pool_internal.go) → contextKey returns "default", nil → the
// error assertions below all fail → RED.
func TestContextKey_DefaultModeWithProxy_Rejected(t *testing.T) {
	const proxy = "http://user:pass@192.0.2.1:9" // TEST-NET-1 — never dialed

	_, err := contextKey("default", proxy)
	if err == nil {
		t.Fatal("contextKey(default, proxy) returned nil error — the proxy would be " +
			"silently dropped and the page would egress on the host's real IP")
	}
	if !errors.Is(err, ErrProxyConflict) {
		t.Errorf("contextKey(default, proxy) error is not ErrProxyConflict: %v", err)
	}
	if code := ClassifyError(err); code != ErrCodeProxyConflict {
		t.Errorf("ClassifyError = %q, want %q — callers must be able to distinguish "+
			"a dropped-proxy rejection", code, ErrCodeProxyConflict)
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error does not name the offending mode: %v", err)
	}
	// The raw proxy URL can carry credentials — it must not leak into the error.
	if strings.Contains(err.Error(), proxy) {
		t.Errorf("error leaks the raw proxy URL (credentials risk): %v", err)
	}

	// All other mode/proxy combinations are unchanged.
	cases := []struct {
		mode    string
		proxy   string
		wantKey string
	}{
		{"default", "", "default"},                        // no proxy → unchanged
		{"proxy", proxy, "proxy:" + proxy},                // proxy mode → unchanged
		{"", proxy, privateProxyKeyPrefix + proxy},        // empty mode + proxy → unchanged
		{"private", proxy, privateProxyKeyPrefix + proxy}, // private + proxy → unchanged
		{"", "", "private"},                               // bare anonymous → unchanged
	}
	for _, tc := range cases {
		t.Run(tc.mode+"/"+tc.wantKey, func(t *testing.T) {
			key, kerr := contextKey(tc.mode, tc.proxy)
			if kerr != nil {
				t.Fatalf("contextKey(%q, %q): unexpected error %v", tc.mode, tc.proxy, kerr)
			}
			if key != tc.wantKey {
				t.Errorf("contextKey(%q, %q) = %q, want %q", tc.mode, tc.proxy, key, tc.wantKey)
			}
		})
	}
}

// TestContextPool_DefaultModeWithProxy_Rejected verifies #97 at the pool
// boundary: GetOrCreatePage with mode "default" + a proxy must fail BEFORE any
// context is created or any page/target is touched. A nil browser is enough —
// the rejection happens before the first CDP call; if the guard were absent the
// pool would instead register a "default" context and proceed to page creation
// on the real IP.
//
// Mutation probe: remove the proxy rejection in contextKey → the call proceeds,
// getOrCreateContextSafe registers a "default" context (nil browser makes
// discovery return "", then page creation fails with ErrUnavailable) →
// len(p.contexts) == 1 and errors.Is(err, ErrProxyConflict) is false → RED.
func TestContextPool_DefaultModeWithProxy_Rejected(t *testing.T) {
	p := NewContextPool(nil)
	defer p.Close()

	_, err := p.GetOrCreatePage("default-proxy-sess", "default",
		"http://user:pass@192.0.2.1:9", "about:blank")
	if err == nil {
		t.Fatal("GetOrCreatePage(default, proxy) returned nil error — " +
			"mode=default+proxy must be rejected, not silently unproxied")
	}
	if !errors.Is(err, ErrProxyConflict) {
		t.Errorf("error is not ErrProxyConflict: %v", err)
	}
	if code := ClassifyError(err); code != ErrCodeProxyConflict {
		t.Errorf("ClassifyError = %q, want %q", code, ErrCodeProxyConflict)
	}

	// No context may exist after the rejection — a registered "default" context
	// here would mean the request proceeded towards a real-IP page.
	p.contextsMu.RLock()
	n := len(p.contexts)
	p.contextsMu.RUnlock()
	if n != 0 {
		t.Errorf("contexts created despite proxy rejection: %d entries "+
			"(the request proceeded on the real IP)", n)
	}
}

// TestRunInteract_DefaultModeWithProxy_Rejected verifies #97 reaches the public
// interact API: every InteractRequest shape that resolves to mode "default"
// plus a non-empty proxy returns Status=error with the proxy_conflict code —
// before any navigation — instead of running unproxied on the host IP.
//
// Mutation probe: remove the proxy rejection in contextKey → each request
// proceeds to context creation + page setup (failing later on the nil browser
// with a backend-unavailable error, not proxy_conflict) → RED.
func TestRunInteract_DefaultModeWithProxy_Rejected(t *testing.T) {
	proxy := "http://user:pass@192.0.2.1:9"
	cases := []struct {
		name string
		req  InteractRequest
	}{
		{"session + explicit default + proxy", InteractRequest{
			URL: "https://example.com/", Session: "s1", Mode: "default", Proxy: &proxy}},
		{"ephemeral default + proxy", InteractRequest{
			URL: "https://example.com/", Mode: "default", Proxy: &proxy}},
		{"use_profile + proxy (backward compat)", InteractRequest{
			URL: "https://example.com/", UseProfile: true, Proxy: &proxy}},
		{"reuse_page + proxy (backward compat)", InteractRequest{
			URL: "https://example.com/", ReusePage: true, Proxy: &proxy}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewContextPool(nil)
			defer p.Close()
			resp := RunInteract(context.Background(), &ChromeManager{pool: p}, tc.req)
			if resp.Status != "error" {
				t.Fatalf("Status = %q, want error — mode=default+proxy must be rejected", resp.Status)
			}
			if resp.ErrorCode != ErrCodeProxyConflict {
				t.Errorf("ErrorCode = %q, want %q", resp.ErrorCode, ErrCodeProxyConflict)
			}
			p.contextsMu.RLock()
			n := len(p.contexts)
			p.contextsMu.RUnlock()
			if n != 0 {
				t.Errorf("contexts created despite proxy rejection: %d entries", n)
			}
		})
	}

	// Control: mode "default" WITHOUT a proxy must sail past the rejection —
	// it fails later on the nil browser with a different code entirely.
	p := NewContextPool(nil)
	defer p.Close()
	resp := RunInteract(context.Background(), &ChromeManager{pool: p}, InteractRequest{
		URL: "https://example.com/", Session: "s2", Mode: "default"})
	if resp.Status != "error" {
		t.Fatalf("control: Status = %q, want error (nil browser)", resp.Status)
	}
	if resp.ErrorCode == ErrCodeProxyConflict {
		t.Errorf("control: mode=default without proxy hit the proxy rejection — guard over-fires")
	}
}

// TestContextKey_ProxyModeWithoutProxy_Rejected verifies the sibling hole of
// #97 at the contextKey level: mode "proxy" with an empty proxy would create a
// direct incognito context on the host's real IP while the caller believes it
// is proxied. It must fail with ErrProxyRequired / proxy_required.
//
// Mutation probe: delete the `if proxy == ""` guard in contextKey's "proxy"
// arm (context_pool_internal.go) → contextKey returns "proxy:", nil → the
// err == nil check below fails → RED.
func TestContextKey_ProxyModeWithoutProxy_Rejected(t *testing.T) {
	_, err := contextKey("proxy", "")
	if err == nil {
		t.Fatal("contextKey(proxy, \"\") returned nil error — the caller asked for " +
			"proxy egress and would silently get a direct context")
	}
	if !errors.Is(err, ErrProxyRequired) {
		t.Errorf("error is not ErrProxyRequired: %v", err)
	}
	if code := ClassifyError(err); code != ErrCodeProxyRequired {
		t.Errorf("ClassifyError = %q, want %q", code, ErrCodeProxyRequired)
	}
}

// TestContextKey_ProxyConflictMessage_ActionableNoEcho pins the conflict
// message: it names the remedy and the implied-by flags, and never echoes the
// (credential-bearing) proxy URL.
func TestContextKey_ProxyConflictMessage_ActionableNoEcho(t *testing.T) {
	const proxy = "http://user:pass@192.0.2.1:9"
	_, err := contextKey("default", proxy)
	if err == nil {
		t.Fatal("expected ErrProxyConflict")
	}
	for _, want := range []string{"use_profile/reuse_page", `mode "proxy"`, `"private"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message lacks %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "pass") || strings.Contains(err.Error(), "192.0.2.1") {
		t.Errorf("message echoes proxy URL parts: %v", err)
	}
}

// TestContextPool_ProxyModeWithoutProxy_Rejected: GetOrCreatePage must fail
// before any context is created, for both an ephemeral-style and a named
// session.
//
// Mutation probe: remove the contextKey guard → the call registers a "proxy:"
// context and fails later (ErrUnavailable) → errors.Is(ErrProxyRequired) false
// and len(p.contexts) == 1 → RED.
func TestContextPool_ProxyModeWithoutProxy_Rejected(t *testing.T) {
	for _, session := range []string{"", "proxy-req-sess"} {
		p := NewContextPool(nil)
		_, err := p.GetOrCreatePage(session, "proxy", "", "about:blank")
		if !errors.Is(err, ErrProxyRequired) {
			t.Errorf("session %q: err = %v, want ErrProxyRequired", session, err)
		}
		p.contextsMu.RLock()
		n := len(p.contexts)
		p.contextsMu.RUnlock()
		if n != 0 {
			t.Errorf("session %q: %d contexts created despite rejection", session, n)
		}
		p.Close()
	}
}

// TestRunInteract_ProxyModeWithoutProxy_Rejected: the public API returns
// proxy_required for mode=proxy with no proxy (ephemeral and named session).
//
// Mutation probe: remove the contextKey guard → the request proceeds to the
// nil browser and fails with a different ErrorCode → RED.
func TestRunInteract_ProxyModeWithoutProxy_Rejected(t *testing.T) {
	for _, req := range []InteractRequest{
		{URL: "https://example.com/", Mode: "proxy"},
		{URL: "https://example.com/", Session: "s", Mode: "proxy"},
	} {
		p := NewContextPool(nil)
		resp := RunInteract(context.Background(), &ChromeManager{pool: p}, req)
		if resp.Status != "error" || resp.ErrorCode != ErrCodeProxyRequired {
			t.Errorf("req %+v: Status=%q ErrorCode=%q, want error/%q",
				req, resp.Status, resp.ErrorCode, ErrCodeProxyRequired)
		}
		p.Close()
	}
}

// TestClassifyError_ProxyBlocked pins the sentinel-table row for the proxy
// guard's refusal.
func TestClassifyError_ProxyBlocked(t *testing.T) {
	err := fmt.Errorf("%w: unparseable proxy URL", ErrProxyBlocked)
	if code := ClassifyError(err); code != ErrCodeProxyBlocked {
		t.Errorf("ClassifyError = %q, want %q", code, ErrCodeProxyBlocked)
	}
}
