package browser

import (
	"errors"
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
		name string
		mode string
		want string
	}{
		{"default", "default", "default"},
		{"private", "private", "private"},
		{"proxy", "proxy", "proxy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := "mode-read-" + tc.name
			mp, err := p.GetOrCreatePage(sess, tc.mode, "", "about:blank")
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

	// Use a non-routable proxy address with credentials so parseProxy returns a
	// sanitized server. about:blank navigation never touches the proxy, so
	// TargetCreateBrowserContext succeeds without a live proxy.
	proxyRaw := "http://user:pass@127.0.0.1:9"
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
