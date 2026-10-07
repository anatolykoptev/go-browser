package browser

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// blockedProxies are caller-supplied proxies that must never reach Chrome.
var blockedProxies = []string{
	"http://127.0.0.1:8765",
	"http://user:pass@127.0.0.1:8765",
	"socks5://10.0.0.5:1080",
	"http://172.18.0.1:8765",
	"http://192.168.1.1:3128",
	"http://169.254.169.254:80",
	"http://100.64.0.1:8080",
	"http://[::1]:8080",
	"http://[fd00::1]:8080",
	"http://0.0.0.0:8080",
	"http://localhost:8080",
	"http://svc.localhost:8080",
	"172.18.0.1:8765", // bare host:port is treated as http
	"ftp://192.0.2.1:21",
	"http://%zz",
	// Chrome parses ProxyServer as proxy RULES: a comma is a fallback list,
	// a semicolon / "=" a per-scheme rule. Each of these would smuggle an
	// unvetted second server past the guard if passed through.
	"http://192.0.2.1:1/,http://127.0.0.1:8765",
	"http://192.0.2.1:1,http://127.0.0.1:8765",
	"http://192.0.2.1:1;https=127.0.0.1:8765",
	"https=127.0.0.1:8765",
	"http://192.0.2.1:1 http://127.0.0.1:8765",
	"http://192.0.2.1:1\thttp://127.0.0.1:8765",
	// Anything beyond scheme://[user:pass@]host[:port].
	"http://192.0.2.1:1/extra",
	"http://192.0.2.1:1?x",
	"http://192.0.2.1:1#frag",
	"http:opaque-192.0.2.1",
	"http://192.0.2.1:99999",
	"http://192.0.2.1:0",
}

// TestNewContext_RefusesInternalProxy exercises the call site in
// ChromeManager.NewContext: a blocked proxy is refused before any CDP work.
//
// Falsification: delete the `if err != nil { return ..., err }` after
// parseProxy in NewContext (chrome_context.go) and the zero-value manager
// returns ErrUnavailable instead of ErrProxyBlocked → RED.
func TestNewContext_RefusesInternalProxy(t *testing.T) {
	m := &ChromeManager{}
	for _, p := range blockedProxies {
		if _, _, _, err := m.NewContext(p); !errors.Is(err, ErrProxyBlocked) {
			t.Errorf("NewContext(%q) err = %v, want ErrProxyBlocked", p, err)
		}
	}
	// A public proxy passes the guard and fails only for lack of a browser.
	if _, _, _, err := m.NewContext("http://u:p@192.0.2.1:8080"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("public proxy: err = %v, want ErrUnavailable", err)
	}
}

// TestGetOrCreateContextSafe_RefusesInternalProxy exercises the pool's
// context-creation call site (the path chrome_interact / render / snapshot /
// screenshot take).
//
// Falsification: delete the parseProxy error return in
// getOrCreateContextSafe (context_pool_internal.go) and the pool goes on to
// a CDP call with no browser → ErrUnavailable, not ErrProxyBlocked → RED.
func TestGetOrCreateContextSafe_RefusesInternalProxy(t *testing.T) {
	p := &ContextPool{contexts: make(map[string]*ManagedContext)}
	for _, raw := range blockedProxies {
		if _, err := p.getOrCreateContextSafe(context.Background(), "k|"+raw, "proxy", raw); !errors.Is(err, ErrProxyBlocked) {
			t.Errorf("getOrCreateContextSafe(%q) err = %v, want ErrProxyBlocked", raw, err)
		}
	}
	if _, err := p.getOrCreateContextSafe(context.Background(), "k|pub", "proxy", "http://192.0.2.1:8080"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("public proxy: err = %v, want ErrUnavailable", err)
	}
}

func TestParseProxy_PublicAndCredentials(t *testing.T) {
	server, user, pass, err := parseProxy(context.Background(), "http://alice:s3cret@192.0.2.1:8080")
	if err != nil {
		t.Fatalf("public proxy refused: %v", err)
	}
	if server != "http://192.0.2.1:8080" || user != "alice" || pass != "s3cret" {
		t.Fatalf("got (%q, %q, %q)", server, user, pass)
	}
	if strings.Contains(server, "s3cret") {
		t.Fatal("credentials leaked into the server string")
	}
	if s, _, _, err := parseProxy(context.Background(), ""); s != "" || err != nil {
		t.Fatalf("empty proxy: (%q, %v), want (\"\", nil)", s, err)
	}
	if s, _, _, err := parseProxy(context.Background(), "192.0.2.1:8080"); err != nil || s != "http://192.0.2.1:8080" {
		t.Fatalf("bare public host:port: (%q, %v)", s, err)
	}
}

// TestParseProxy_Allowlist: EGRESS_PROXY_ALLOW admits named internal proxies
// (host or host:port) and nothing else.
func TestParseProxy_Allowlist(t *testing.T) {
	orig := proxyAllow
	t.Cleanup(func() { proxyAllow = orig })
	proxyAllow = parseHostList(" Tor , 172.18.0.1:1082 ")

	for _, ok := range []string{"socks5://tor:9050", "http://172.18.0.1:1082"} {
		if _, _, _, err := parseProxy(context.Background(), ok); err != nil {
			t.Errorf("allowlisted %q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://172.18.0.1:8765",
		"http://127.0.0.1:9050",
		// Allowlisted entries go through the same syntax checks.
		"socks5://tor:9050,http://127.0.0.1:8765",
		"http://172.18.0.1:1082/x",
		"http://172.18.0.1:1082;http=127.0.0.1:1",
	} {
		if _, _, _, err := parseProxy(context.Background(), bad); !errors.Is(err, ErrProxyBlocked) {
			t.Errorf("%q: err = %v, want ErrProxyBlocked (only the listed host:port is allowed)", bad, err)
		}
	}
}

// TestProxyCredentials_NoDNS: the interact path recovers proxy credentials
// for an already-vetted proxy. It must not depend on DNS, or a lookup
// failure would silently skip proxy auth (Chrome then gets 407).
//
// Falsification: make proxyCredentials delegate to parseProxy (the vetting,
// DNS-resolving parser) and the unresolvable host loses its credentials →
// RED. The interact.go call site itself needs a live Chrome to exercise.
func TestProxyCredentials_NoDNS(t *testing.T) {
	user, pass := proxyCredentials("http://alice:s3cret@unresolvable.invalid:80")
	if user != "alice" || pass != "s3cret" {
		t.Fatalf("got (%q, %q), want (alice, s3cret)", user, pass)
	}
	if u, p := proxyCredentials("http://host.invalid:80"); u != "" || p != "" {
		t.Fatalf("no-credential proxy: got (%q, %q)", u, p)
	}
	if _, _, _, err := parseProxy(context.Background(), "http://alice:s3cret@unresolvable.invalid:80"); !errors.Is(err, ErrProxyBlocked) {
		t.Fatalf("parseProxy must still refuse an unresolvable host at context creation, got %v", err)
	}
}

// TestParseProxy_ServerRebuiltFromParts: the ProxyServer value Chrome gets is
// rebuilt from scheme + host[:port], never copied from the caller. A trailing
// "/" is dropped so the proxy still applies (Chrome would not accept
// "http://host:port/" as a proxy server and would egress direct).
//
// Falsification: return u.String() from parseProxy again and the trailing
// slash survives → RED.
func TestParseProxy_ServerRebuiltFromParts(t *testing.T) {
	for raw, want := range map[string]string{
		"http://alice:pw@192.0.2.1:8080/": "http://192.0.2.1:8080",
		"http://192.0.2.1:8080":           "http://192.0.2.1:8080",
		"socks5://192.0.2.1:1080/":        "socks5://192.0.2.1:1080",
		"http://192.0.2.1":                "http://192.0.2.1",
		"http://[2001:db8::1]:3128/":      "http://[2001:db8::1]:3128",
	} {
		got, _, _, err := parseProxy(context.Background(), raw)
		if err != nil || got != want {
			t.Errorf("parseProxy(%q) = (%q, %v), want %q", raw, got, err, want)
		}
	}
	if _, u, p, _ := parseProxy(context.Background(), "http://alice:pw@192.0.2.1:8080/"); u != "alice" || p != "pw" {
		t.Errorf("credentials lost with trailing slash: (%q, %q)", u, p)
	}
}

// TestParseProxy_HonoursCallerContext: a cancelled request context stops the
// proxy-host lookup instead of running the full checkTimeout.
func TestParseProxy_HonoursCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := parseProxy(ctx, "http://example.com:8080"); !errors.Is(err, ErrProxyBlocked) {
		t.Fatalf("cancelled ctx: err = %v, want ErrProxyBlocked (lookup refused)", err)
	}
}

// TestParseProxy_RulesSyntaxRefusedUpFront: proxy-rules characters and
// whitespace are refused before any parsing or DNS work. (The structural
// checks and the rebuild from parts would also stop every smuggling row in
// blockedProxies; this check is the explicit first line, and it must not be
// the allowlist that decides.)
//
// Falsification: drop the ContainsAny/IndexFunc check in parseProxy and
// these inputs fail later with a different reason → RED.
func TestParseProxy_RulesSyntaxRefusedUpFront(t *testing.T) {
	orig := proxyAllow
	t.Cleanup(func() { proxyAllow = orig })
	proxyAllow = parseHostList("tor:9050")
	for _, raw := range []string{
		"http://192.0.2.1:1,http://127.0.0.1:8765",
		"http://192.0.2.1:1;https=127.0.0.1:8765",
		"socks5://tor:9050,http://127.0.0.1:8765",
		"http://user=x:pw@192.0.2.1:8080",
		"http://192.0.2.1:1 http://127.0.0.1:8765",
	} {
		_, _, _, err := parseProxy(context.Background(), raw)
		if !errors.Is(err, ErrProxyBlocked) || !strings.Contains(err.Error(), "proxy-rules syntax") {
			t.Errorf("parseProxy(%q) err = %v, want the proxy-rules syntax refusal", raw, err)
		}
	}
}
