package browser

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
		"":                                  "",
		"http://proxy.example.net:80":       "http://proxy.example.net:80",
		"http://u:p@proxy.example.net:80":   "http://proxy.example.net:80",
		"socks5://u@10.0.0.1:1080":          "socks5://10.0.0.1:1080",
		"u:p@proxy.example.net:80":          "proxy.example.net:80",
		"http://u:p%40x@[2001:db8::1]:3128": "http://[2001:db8::1]:3128",
	}
	for in, want := range cases {
		if got := redactProxyUserinfo(in); got != want {
			t.Errorf("redactProxyUserinfo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAuthChallengeResponse_OnlyProxyGetsCredentials(t *testing.T) {
	provide := proto.FetchAuthChallengeResponseResponseProvideCredentials
	cancel := proto.FetchAuthChallengeResponseResponseCancelAuth
	cases := []struct {
		name   string
		ch     *proto.FetchAuthChallenge
		active bool
		want   proto.FetchAuthChallengeResponseResponse
	}{
		{"proxy challenge, creds registered", &proto.FetchAuthChallenge{Source: proto.FetchAuthChallengeSourceProxy}, true, provide},
		{"server challenge, creds registered", &proto.FetchAuthChallenge{Source: proto.FetchAuthChallengeSourceServer}, true, cancel},
		{"no source, creds registered", &proto.FetchAuthChallenge{}, true, cancel},
		{"nil challenge, creds registered", nil, true, cancel},
		{"proxy challenge, nothing registered", &proto.FetchAuthChallenge{Source: proto.FetchAuthChallengeSourceProxy}, false, cancel},
	}
	for _, c := range cases {
		r := authChallengeResponse(c.ch, c.active, "u", "p")
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
	unregister := guard.registerProxyAuth(user, pass)
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
