package browser

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// TestContextPool_ListRedactsProxyUserinfo drives the production List path
// (what chrome_tabs serializes) with a context whose proxy carries
// credentials, and requires that none of them reach ContextInfo.
func TestContextPool_ListRedactsProxyUserinfo(t *testing.T) {
	const raw = "http://alice:s3cret-pw@proxy.example.net:8080"
	p := &ContextPool{contexts: map[string]*ManagedContext{
		"proxy:" + raw: {Mode: modeProxy, Proxy: raw, Pages: map[string]*ManagedPage{}},
	}}
	infos := p.List()
	if len(infos) != 1 {
		t.Fatalf("List() returned %d contexts, want 1", len(infos))
	}
	got := infos[0].Proxy
	for _, secret := range []string{"alice", "s3cret-pw", "@"} {
		if strings.Contains(got, secret) {
			t.Fatalf("ContextInfo.Proxy = %q leaks %q", got, secret)
		}
	}
	if got != "http://proxy.example.net:8080" {
		t.Errorf("ContextInfo.Proxy = %q, want http://proxy.example.net:8080", got)
	}
}

func TestRedactProxyUserinfo(t *testing.T) {
	cases := map[string]string{
		"":                                   "",
		"http://proxy.example.net:80":        "http://proxy.example.net:80",
		"http://u:p@proxy.example.net:80":    "http://proxy.example.net:80",
		"HTTP://u:p@proxy.example.net":       "HTTP://proxy.example.net",
		"socks5://u@10.0.0.1:1080":           "socks5://10.0.0.1:1080",
		"u:p@proxy.example.net:80":           "proxy.example.net:80",
		"http://u:p%40x@[2001:db8::1]:3128":  "http://[2001:db8::1]:3128",
		"u:p@[2001:db8::1]:3128":             "[2001:db8::1]:3128",
		"http://u:p@ss@proxy.example.net:80": "http://proxy.example.net:80",
		// url.Parse would read host "u:1234" and the rest as a fragment.
		"http://u:1234#x@proxy.example.net:80": "http://proxy.example.net:80",
	}
	for in, want := range cases {
		if got := redactProxyUserinfo(in); got != want {
			t.Errorf("redactProxyUserinfo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactContextKey(t *testing.T) {
	cases := map[string]string{
		"default":                        "default",
		"private":                        "private",
		"proxy:http://u:pw@1.2.3.4:8080": "proxy:http://1.2.3.4:8080",
		"proxy:http://1.2.3.4:8080":      "proxy:http://1.2.3.4:8080",
	}
	for in, want := range cases {
		if got := redactContextKey(in); got != want {
			t.Errorf("redactContextKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSameProxyOrigin(t *testing.T) {
	cases := []struct {
		origin, server string
		want           bool
	}{
		{"http://1.2.3.4:8080", "http://1.2.3.4:8080", true},
		{"http://1.2.3.4", "http://1.2.3.4:80", true},
		{"http://1.2.3.4:80", "http://1.2.3.4", true},
		{"HTTPS://Proxy.Example.net", "https://proxy.example.net:443", true},
		{"http://[2001:db8::1]:3128", "http://[2001:db8::1]:3128", true},
		{"http://5.6.7.8:8080", "http://1.2.3.4:8080", false},
		{"http://1.2.3.4:8081", "http://1.2.3.4:8080", false},
		{"https://1.2.3.4:8080", "http://1.2.3.4:8080", false},
		{"", "http://1.2.3.4:8080", false},
		{"http://1.2.3.4:8080", "", false},
		{"http://u:p@1.2.3.4:8080", "http://1.2.3.4:8080", false},
	}
	for _, c := range cases {
		if got := sameProxyOrigin(c.origin, c.server); got != c.want {
			t.Errorf("sameProxyOrigin(%q, %q) = %v, want %v", c.origin, c.server, got, c.want)
		}
	}
}

func TestAuthChallengeResponse_OnlyRegisteredProxyGetsCredentials(t *testing.T) {
	const server = "http://1.2.3.4:8080"
	provide := proto.FetchAuthChallengeResponseResponseProvideCredentials
	cancel := proto.FetchAuthChallengeResponseResponseCancelAuth
	proxyCh := func(origin string) *proto.FetchAuthChallenge {
		return &proto.FetchAuthChallenge{Source: proto.FetchAuthChallengeSourceProxy, Origin: origin}
	}
	cases := []struct {
		name   string
		ch     *proto.FetchAuthChallenge
		active bool
		want   proto.FetchAuthChallengeResponseResponse
	}{
		{"registered proxy", proxyCh(server), true, provide},
		{"another proxy", proxyCh("http://5.6.7.8:8080"), true, cancel},
		{"proxy challenge, no origin", proxyCh(""), true, cancel},
		{"server challenge from the same origin", &proto.FetchAuthChallenge{Source: proto.FetchAuthChallengeSourceServer, Origin: server}, true, cancel},
		{"no source", &proto.FetchAuthChallenge{Origin: server}, true, cancel},
		{"nil challenge", nil, true, cancel},
		{"nothing registered", proxyCh(server), false, cancel},
	}
	for _, c := range cases {
		r := authChallengeResponse(c.ch, c.active, server, "u", "p")
		if r.Response != c.want {
			t.Errorf("%s: response = %q, want %q", c.name, r.Response, c.want)
		}
		if r.Response != provide && (r.Username != "" || r.Password != "") {
			t.Errorf("%s: credentials set on a %q response", c.name, r.Response)
		}
	}
}

// TestEgressGuard_SiteBasicChallenge_GetsNoProxyCredentials is the call-site
// check: with proxy credentials registered on the shared guard, a site that
// answers 401 + WWW-Authenticate: Basic must never receive them in an
// Authorization header. Before the source check, respondAuth answered the
// site's Server-sourced challenge with the proxy credentials.
func TestEgressGuard_SiteBasicChallenge_GetsNoProxyCredentials(t *testing.T) {
	b := acquireSharedBrowser(t)
	guard := acquireGuard(t, b)

	const user, pass = "proxy-user-x", "proxy-pass-x"
	unregister := guard.registerProxyAuth("http://203.0.113.10:3128", user, pass)
	defer unregister()

	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("WWW-Authenticate", `Basic realm="site"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	// The test server is on loopback, which the guard blocks by default.
	prev := allowLocalhost
	allowLocalhost = true
	t.Cleanup(func() { allowLocalhost = prev })

	page, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	defer func() { _ = page.Close() }()
	_ = page.Timeout(navigateTimeout).Navigate(srv.URL + "/?basic-challenge=1")
	_ = page.Timeout(navigateTimeout).WaitLoad()

	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("site never saw a request; the test did not exercise the challenge")
	}
	leaked := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	for _, h := range seen {
		if h == leaked {
			t.Fatalf("site received the proxy credentials in Authorization: %q", h)
		}
	}
}

// fakeAuthProxy is a forward proxy that demands Basic proxy auth: it answers
// 407 until a request carries Proxy-Authorization, and records every header
// it sees.
func fakeAuthProxy(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Proxy-Authorization")
		mu.Lock()
		seen = append(seen, h)
		mu.Unlock()
		if h == "" {
			w.Header().Set("Proxy-Authenticate", `Basic realm="proxy"`)
			w.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
		_, _ = w.Write([]byte("proxied-ok"))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

// navigateThroughProxy opens a page in a fresh browser context whose
// ProxyServer is proxyURL and navigates to a public IP literal (no DNS, so
// the egress guard lets it through and Chrome sends it to the proxy).
func navigateThroughProxy(t *testing.T, proxyURL string) {
	t.Helper()
	b := acquireSharedBrowser(t)
	acquireGuard(t, b)
	res, err := proto.TargetCreateBrowserContext{ProxyServer: proxyURL, DisposeOnDetach: true}.Call(b)
	if err != nil {
		t.Fatalf("create browser context: %v", err)
	}
	t.Cleanup(func() { _ = proto.TargetDisposeBrowserContext{BrowserContextID: res.BrowserContextID}.Call(b) })
	// rod's Browser.Page overrides BrowserContextID with the browser's own,
	// so scope a browser to the new context (as chrome_context.go does).
	scoped := b.NoDefaultDevice()
	scoped.BrowserContextID = res.BrowserContextID
	page, err := scoped.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	t.Cleanup(func() { _ = page.Close() })
	if err := page.Timeout(navigateTimeout).Navigate("http://example.com/?proxy-auth-test=1"); err != nil {
		t.Logf("navigate: %v", err)
	}

}

func hasCreds(seen []string, user, pass string) bool {
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	for _, h := range seen {
		if h == want {
			return true
		}
	}
	return false
}

// TestEgressGuard_RegisteredProxy_GetsCredentials is the positive path that
// threads-mcp depends on: the proxy the credentials were registered for gets
// them on its 407.
func TestEgressGuard_RegisteredProxy_GetsCredentials(t *testing.T) {
	proxy, seen := fakeAuthProxy(t)
	guard := acquireGuard(t, acquireSharedBrowser(t))
	unregister := guard.registerProxyAuth(proxy.URL, "own-user", "own-pass")
	defer unregister()

	navigateThroughProxy(t, proxy.URL)

	got := seen()
	if len(got) == 0 {
		t.Fatal("proxy never saw a request; the test did not exercise the challenge")
	}
	if !hasCreds(got, "own-user", "own-pass") {
		t.Fatalf("registered proxy never received its credentials; Proxy-Authorization seen: %q", got)
	}
}

// TestEgressGuard_ForeignProxy_GetsNoCredentials: credentials registered for
// one proxy must not go to a different proxy that also answers 407 (proxies
// are caller supplied, and the credential slot is connection-wide).
func TestEgressGuard_ForeignProxy_GetsNoCredentials(t *testing.T) {
	proxy, seen := fakeAuthProxy(t)
	guard := acquireGuard(t, acquireSharedBrowser(t))
	unregister := guard.registerProxyAuth("http://203.0.113.10:3128", "victim-user", "victim-pass")
	defer unregister()

	navigateThroughProxy(t, proxy.URL)

	got := seen()
	if len(got) == 0 {
		t.Fatal("proxy never saw a request; the test did not exercise the challenge")
	}
	if hasCreds(got, "victim-user", "victim-pass") {
		t.Fatalf("a foreign proxy received another proxy's credentials: %q", got)
	}
}

func TestAuthChallengeResponse_DefaultPortElided(t *testing.T) {
	r := authChallengeResponse(&proto.FetchAuthChallenge{Source: proto.FetchAuthChallengeSourceProxy, Origin: "http://1.2.3.4"}, true, "http://1.2.3.4:80", "u", "p")
	if r.Response != proto.FetchAuthChallengeResponseResponseProvideCredentials {
		t.Fatalf("origin without its default port: response = %q, want ProvideCredentials", r.Response)
	}
}

// allowLoopbackProxies lets parseProxy accept the loopback fake proxies.
func allowLoopbackProxies(t *testing.T) {
	t.Helper()
	prev := proxyAllow
	proxyAllow = map[string]struct{}{"127.0.0.1": {}}
	t.Cleanup(func() { proxyAllow = prev })
}

func interactChrome(t *testing.T) *ChromeManager {
	t.Helper()
	br := acquireSharedBrowser(t)
	guard := acquireGuard(t, br)
	pool := NewContextPool(br)
	t.Cleanup(pool.Close)
	return &ChromeManager{pool: pool, browser: br, guard: guard}
}

// TestRunInteract_ProxyMode_RegistersCredentialsForItsProxy drives the
// production wiring (RunInteract -> pool -> ManagedPage.ProxyServer ->
// registerProxyAuth) with a credentialed proxy and requires the proxy to
// receive its credentials.
func TestRunInteract_ProxyMode_RegistersCredentialsForItsProxy(t *testing.T) {
	allowLoopbackProxies(t)
	chrome := interactChrome(t)
	proxy, seen := fakeAuthProxy(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	raw := "http://own-user:own-pass@" + proxy.Listener.Addr().String()
	resp := RunInteract(ctx, chrome, InteractRequest{Mode: modeProxy, Proxy: &raw, NoStealth: true, URL: "http://example.com/?wiring=1"})
	t.Logf("interact: %s %s", resp.Status, resp.Error)
	if !hasCreds(seen(), "own-user", "own-pass") {
		t.Fatalf("proxy never received its credentials via RunInteract; saw %q", seen())
	}
}

// TestRunInteract_SharedPrivateContext_RefusesOtherProxy: "private" is keyed
// without the proxy, so a second caller with its own proxy lands in a
// context that dials the first caller's proxy. It must be refused, and its
// credentials must never reach the first caller's proxy.
func TestRunInteract_SharedPrivateContext_RefusesOtherProxy(t *testing.T) {
	allowLoopbackProxies(t)
	chrome := interactChrome(t)
	first, firstSeen := fakeAuthProxy(t)
	second, secondSeen := fakeAuthProxy(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	firstURL := "http://first:first-pass@" + first.Listener.Addr().String()
	r1 := RunInteract(ctx, chrome, InteractRequest{Session: "first-sess", Mode: "private", Proxy: &firstURL, NoStealth: true, URL: "about:blank"})
	if r1.Status == "error" {
		t.Fatalf("first caller: %s", r1.Error)
	}
	secondURL := "http://second:second-pass@" + second.Listener.Addr().String()
	r2 := RunInteract(ctx, chrome, InteractRequest{Mode: "private", Proxy: &secondURL, NoStealth: true, URL: "http://example.com/?collide=1"})
	if r2.Status != "error" || !strings.Contains(r2.Error, "different proxy") {
		t.Errorf("second caller: status %q error %q, want refusal", r2.Status, r2.Error)
	}
	if hasCreds(firstSeen(), "second", "second-pass") {
		t.Fatalf("second caller's credentials reached the first caller's proxy: %q", firstSeen())
	}
	_ = secondSeen
}
