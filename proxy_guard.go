package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

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

// parseProxy validates a proxy URL and splits out its credentials. It is the
// single place a caller-supplied proxy is turned into Chrome's ProxyServer
// value, so every BrowserContext creation goes through the same check.
//
// Input:  "http://user:pass@host:port" → ("http://<ip>:port", "user", "pass")
// Input:  ""                           → ("", "", "", nil)
//
// Rules:
//   - scheme must be http, https or socks5 (a bare host:port means http);
//   - the host is resolved and EVERY address must be public
//     (httputil.IsBlockedIP), unless the host is on EGRESS_PROXY_ALLOW;
//   - for http and socks5 the returned server is pinned to the vetted IP, so
//     a DNS answer that changes between this check and Chrome's own lookup
//     cannot redirect the proxy connection. https keeps the hostname (TLS
//     to the proxy needs it).
func parseProxy(raw string) (server, user, pass string, err error) {
	if raw == "" {
		return "", "", "", nil
	}
	toParse := raw
	if !strings.Contains(raw, "://") {
		toParse = "http://" + raw
	}
	u, perr := url.Parse(toParse)
	if perr != nil || u.Hostname() == "" {
		return "", "", "", fmt.Errorf("%w: unparseable proxy URL", ErrProxyBlocked)
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return "", "", "", fmt.Errorf("%w: unsupported proxy scheme %q", ErrProxyBlocked, u.Scheme)
	}
	if u.User != nil {
		pass, _ = u.User.Password()
		user = u.User.Username()
		u.User = nil
	}
	if !proxyAllowed(u) {
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		ip, verr := vetProxyHost(ctx, u.Hostname())
		if verr != nil {
			return "", "", "", verr
		}
		if u.Scheme != "https" {
			host := ip.String()
			if ip.To4() == nil {
				host = "[" + host + "]"
			}
			if port := u.Port(); port != "" {
				host = net.JoinHostPort(ip.String(), port)
			}
			u.Host = host
		}
	}
	server = u.String()
	if !strings.Contains(raw, "://") {
		server = strings.TrimPrefix(server, "http://")
	}
	return server, user, pass, nil
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
