package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/anatolykoptev/go-kit/httputil"
)

// ErrProxyBlocked means a caller-supplied proxy URL was refused: malformed,
// an unsupported scheme, or a host that resolves to a loopback, private,
// link-local, CGNAT, ULA or unspecified address and is not on the
// EGRESS_PROXY_ALLOW list.
//
// The egress guard (egress_guard.go) checks the TARGET of every Chrome
// request, but Chrome connects to the proxy itself without the guard seeing
// it. Without this check a caller could name an internal service as its
// "proxy" and have Chrome send traffic there even with
// EGRESS_ALLOW_LOCALHOST off.
var ErrProxyBlocked = errors.New("browser: proxy refused")

// proxyAllowEnv lists proxy hosts (host or host:port, comma-separated) that
// may resolve to internal addresses — e.g. a sidecar proxy on the docker
// network. Matched against the host as written in the proxy URL.
const proxyAllowEnv = "EGRESS_PROXY_ALLOW"

// proxyAllow is parsed once at init, like allowLocalhost.
var proxyAllow = parseHostList(os.Getenv(proxyAllowEnv))

func parseHostList(csv string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, h := range strings.Split(csv, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			out[h] = struct{}{}
		}
	}
	return out
}

// proxyAllowed reports whether u's host (or host:port) is on the allowlist.
func proxyAllowed(u *url.URL) bool {
	if _, ok := proxyAllow[strings.ToLower(u.Hostname())]; ok {
		return true
	}
	_, ok := proxyAllow[strings.ToLower(u.Host)]
	return ok
}

// proxyRuleChars are characters with meaning in Chrome's proxy-rules
// syntax (TargetCreateBrowserContext.ProxyServer is parsed as rules, not as
// one URL): "," separates fallbacks, ";" separates per-scheme rules, "="
// binds a scheme to a server. Any of them, or whitespace, in a caller's
// proxy could smuggle a second, unvetted server into the rules.
const proxyRuleChars = ",;="

// parseProxy validates a proxy URL and splits out its credentials. It is the
// single place a caller-supplied proxy is turned into Chrome's ProxyServer
// value, so every BrowserContext creation goes through the same check.
//
// Input:  "http://user:pass@host:port" → ("http://<ip>:port", "user", "pass")
// Input:  ""                           → ("", "", "", nil)
//
// Rules:
//   - no proxy-rules syntax (",", ";", "=") and no whitespace anywhere;
//   - scheme http, https or socks5 (a bare host:port means http);
//   - no opaque part, query, fragment or path; a lone trailing "/" is
//     dropped (Chrome would not accept it as a proxy server);
//   - port, if present, is 1-65535;
//   - the host is resolved and EVERY address must be public
//     (httputil.IsBlockedIP), unless the host is on EGRESS_PROXY_ALLOW;
//   - the returned server is rebuilt from scheme + host[:port] only, never
//     from the caller's string. For http and socks5 the host is the vetted
//     IP, so a DNS answer that changes between this check and Chrome's own
//     lookup cannot redirect the proxy connection.
//
// Residual (documented, not closed): an https proxy keeps its hostname (TLS
// to the proxy needs it), and an allowlisted host is not resolved or pinned,
// so for those two cases Chrome's own DNS lookup decides the address — a
// DNS-rebinding window remains. Neither is used in the fleet today.
//
// ctx bounds the DNS lookup (together with checkTimeout).
func parseProxy(ctx context.Context, raw string) (server, user, pass string, err error) {
	if raw == "" {
		return "", "", "", nil
	}
	if strings.ContainsAny(raw, proxyRuleChars) || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return "", "", "", fmt.Errorf("%w: proxy URL contains proxy-rules syntax or whitespace", ErrProxyBlocked)
	}
	toParse := raw
	if !strings.Contains(raw, "://") {
		toParse = "http://" + raw
	}
	u, perr := url.Parse(toParse)
	if perr != nil || u.Opaque != "" || u.Hostname() == "" {
		return "", "", "", fmt.Errorf("%w: unparseable proxy URL", ErrProxyBlocked)
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return "", "", "", fmt.Errorf("%w: unsupported proxy scheme %q", ErrProxyBlocked, u.Scheme)
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" ||
		(u.Path != "" && u.Path != "/") {
		return "", "", "", fmt.Errorf("%w: proxy URL must be scheme://[user:pass@]host[:port]", ErrProxyBlocked)
	}
	port := u.Port()
	if port != "" {
		if n, perr := strconv.Atoi(port); perr != nil || n < 1 || n > 65535 {
			return "", "", "", fmt.Errorf("%w: invalid proxy port", ErrProxyBlocked)
		}
	}
	if u.User != nil {
		pass, _ = u.User.Password()
		user = u.User.Username()
	}

	host := u.Hostname()
	if !proxyAllowed(u) {
		vctx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		ip, verr := vetProxyHost(vctx, host)
		if verr != nil {
			return "", "", "", verr
		}
		if u.Scheme != "https" {
			host = ip.String()
		}
	}
	return u.Scheme + "://" + joinHostPort(host, port), user, pass, nil
}

// joinHostPort is net.JoinHostPort that tolerates an empty port.
func joinHostPort(host, port string) string {
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

// proxyCredentials returns the user and password embedded in a proxy URL
// without validating or resolving it. Use it only for a proxy that already
// passed parseProxy (at context creation): it must not fail on a transient
// DNS error, or proxy auth would be skipped and Chrome would get 407.
func proxyCredentials(raw string) (user, pass string) {
	if raw == "" {
		return "", ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return "", ""
	}
	pass, _ = u.User.Password()
	return u.User.Username(), pass
}

// vetProxyHost resolves host and returns the first address when every
// address is public. Any blocked address, or a resolution failure, refuses
// the proxy: fail closed.
func vetProxyHost(ctx context.Context, host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if httputil.IsBlockedIP(ip) {
			return nil, fmt.Errorf("%w: proxy host %s is a non-public address", ErrProxyBlocked, ip)
		}
		return ip, nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, fmt.Errorf("%w: proxy host %s is loopback", ErrProxyBlocked, host)
	}
	resolver := net.DefaultResolver
	if r, ok := ctx.Value(resolverKey{}).(*net.Resolver); ok && r != nil {
		resolver = r
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("%w: cannot resolve proxy host %s", ErrProxyBlocked, host)
	}
	for _, a := range addrs {
		if httputil.IsBlockedIP(a.IP) {
			return nil, fmt.Errorf("%w: proxy host %s resolves to non-public %s", ErrProxyBlocked, host, a.IP)
		}
	}
	return addrs[0].IP, nil
}

// redactProxyUserinfo returns raw without its userinfo, for anything that
// leaves the process (ContextInfo.Proxy is served by chrome_tabs). It cuts
// everything between the scheme and the LAST '@' as plain text rather than
// trusting a URL parser: a malformed value (e.g. a '#' inside the password)
// can make url.Parse place the host inside the credentials, but no parse can
// move the last '@'.
func redactProxyUserinfo(raw string) string {
	at := strings.LastIndex(raw, "@")
	if at < 0 {
		return raw
	}
	if sep := strings.Index(raw, "://"); sep >= 0 && sep < at {
		return raw[:sep+3] + raw[at+1:]
	}
	return raw[at+1:]
}

// redactContextKey is a pool key ("default" | "private" |
// "private-proxy:<raw>" | "proxy:<raw>") with any proxy credentials removed,
// for errors and logs.
func redactContextKey(key string) string {
	for _, prefix := range []string{privateProxyKeyPrefix, "proxy:"} {
		if rest, ok := strings.CutPrefix(key, prefix); ok {
			return prefix + redactProxyUserinfo(rest)
		}
	}
	return key
}

// defaultProxyPorts maps a proxy scheme to the port Chrome uses when none is
// given. Chrome serializes a challenger origin without its default port.
var defaultProxyPorts = map[string]string{"http": "80", "https": "443", "socks5": "1080"}

// sameProxyOrigin reports whether a CDP auth challenger origin names the
// proxy server the credentials were registered for. Both sides are compared
// as scheme + host + effective port, case-insensitively, so "http://1.2.3.4"
// and "http://1.2.3.4:80" match. Empty or unparsable input never matches.
func sameProxyOrigin(origin, server string) bool {
	o, okO := proxyOriginKey(origin)
	s, okS := proxyOriginKey(server)
	return okO && okS && o == s
}

func proxyOriginKey(v string) (string, bool) {
	if v == "" {
		return "", false
	}
	if !strings.Contains(v, "://") {
		v = "http://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		port = defaultProxyPorts[scheme]
	}
	if port == "" {
		return "", false
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port), true
}
