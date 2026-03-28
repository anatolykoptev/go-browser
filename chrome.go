package browser

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/ysmood/gson"
)

//go:embed stealth_complement.js
var complementJS string

// ChromeManager manages a connection to a remote Chrome instance (CloakBrowser).
type ChromeManager struct {
	mu             sync.RWMutex
	browser        *rod.Browser
	wsURL          string                        // original ws URL for discovery
	keepaliveCtxID proto.BrowserBrowserContextID // prevents Chrome exit when all user contexts close
}

// NewChromeManager connects to a remote Chrome via WebSocket debugger URL.
// wsURL may be a ws:// address; the actual debugger URL is discovered via /json/version.
// A keepalive browser context is created to prevent headless Chrome from exiting
// when all user contexts are disposed.
func NewChromeManager(wsURL string) (*ChromeManager, error) {
	debuggerURL, err := discoverWSURL(wsURL)
	if err != nil {
		return nil, fmt.Errorf("chrome: discover ws url: %w", err)
	}

	b := rod.New().ControlURL(debuggerURL)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("chrome: connect: %w", err)
	}

	// Close any pre-existing pages (e.g. chrome://newtab) that CloakBrowser creates on startup.
	// These can cause crashes when mixed with programmatic context lifecycle.
	closeInitialPages(b)

	// Create a keepalive context so Chrome doesn't exit when user contexts are disposed.
	// Non-fatal: headed Chrome with Xvfb doesn't need keepalive.
	keepaliveCtxID, err := createKeepaliveContext(b)
	if err != nil {
		// Don't fail — headed Chrome survives without keepalive
		keepaliveCtxID = ""
	}

	return &ChromeManager{
		browser:        b,
		wsURL:          wsURL,
		keepaliveCtxID: keepaliveCtxID,
	}, nil
}

// closeInitialPages closes any pre-existing pages (e.g. chrome://newtab) created by
// CloakBrowser on startup. These default pages can interfere with context lifecycle.
func closeInitialPages(b *rod.Browser) {
	targets, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		return
	}
	for _, t := range targets.TargetInfos {
		if t.Type == "page" {
			_, _ = proto.TargetCloseTarget{TargetID: t.TargetID}.Call(b)
		}
	}
}

// createKeepaliveContext creates a browser context with a blank page that prevents
// headless Chrome from terminating when all other contexts/pages are closed.
func createKeepaliveContext(b *rod.Browser) (proto.BrowserBrowserContextID, error) {
	res, err := proto.TargetCreateBrowserContext{}.Call(b)
	if err != nil {
		return "", fmt.Errorf("create keepalive context: %w", err)
	}

	// Create a page inside the context — Chrome needs at least one target to stay alive.
	_, err = proto.TargetCreateTarget{
		URL:              "about:blank",
		BrowserContextID: res.BrowserContextID,
	}.Call(b)
	if err != nil {
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: res.BrowserContextID}.Call(b)
		return "", fmt.Errorf("create keepalive page: %w", err)
	}

	return res.BrowserContextID, nil
}

// DefaultContext returns the browser's default context (persistent profile).
// Cookies, localStorage, and other state from manual login sessions are available.
// No isolation — all requests share the same cookies.
func (m *ChromeManager) DefaultContext() (*rod.Browser, error) {
	b := m.getBrowser()
	if b == nil {
		return nil, ErrUnavailable
	}
	scoped := b.NoDefaultDevice()
	scoped.BrowserContextID = "" // empty = default context (persistent profile)
	return scoped, nil
}

// NewContext creates an isolated BrowserContext with optional proxy.
// Returns a browser scoped to that context, the context ID for lifecycle management,
// and a cleanup function for proxy auth handling.
// Supports authenticated proxies (http://user:pass@host:port) via CDP Fetch.authRequired.
func (m *ChromeManager) NewContext(proxy string) (*rod.Browser, proto.BrowserBrowserContextID, func(), error) {
	b := m.getBrowser()
	if b == nil {
		return nil, "", nil, ErrUnavailable
	}

	proxyServer, proxyUser, proxyPass := parseProxy(proxy)

	createCtx := func(browser *rod.Browser) (*proto.TargetCreateBrowserContextResult, error) {
		return proto.TargetCreateBrowserContext{
			ProxyServer:     proxyServer,
			DisposeOnDetach: true,
		}.Call(browser)
	}

	res, err := createCtx(b)
	if err != nil {
		if reconnErr := m.reconnect(); reconnErr != nil {
			return nil, "", nil, fmt.Errorf("chrome: create browser context: %w (reconnect failed: %v)", err, reconnErr)
		}
		b = m.getBrowser()
		if b == nil {
			return nil, "", nil, fmt.Errorf("chrome: reconnect succeeded but browser is nil")
		}
		res, err = createCtx(b)
		if err != nil {
			return nil, "", nil, fmt.Errorf("chrome: create browser context after reconnect: %w", err)
		}
	}

	scoped := b.NoDefaultDevice()
	scoped.BrowserContextID = res.BrowserContextID

	// Set up continuous proxy auth handler if credentials provided.
	var cleanup func()
	if proxyUser != "" {
		cleanup = setupProxyAuth(scoped, proxyUser, proxyPass)
	}

	return scoped, res.BrowserContextID, cleanup, nil
}

// setupProxyAuth enables continuous Fetch.authRequired handling for proxy authentication.
// Intercepts all requests but immediately continues non-auth ones in goroutines
// to avoid blocking page XHR. Auth challenges get credentials.
// Returns a cleanup function that disables the fetch domain.
func setupProxyAuth(b *rod.Browser, username, password string) func() {
	_ = proto.FetchEnable{
		HandleAuthRequests: true,
	}.Call(b)

	wait := b.EachEvent(
		func(ev *proto.FetchRequestPaused) {
			// Continue immediately in goroutine — don't block the event loop.
			go func() {
				_ = proto.FetchContinueRequest{RequestID: ev.RequestID}.Call(b)
			}()
		},
		func(ev *proto.FetchAuthRequired) {
			go func() {
				_ = proto.FetchContinueWithAuth{
					RequestID: ev.RequestID,
					AuthChallengeResponse: &proto.FetchAuthChallengeResponse{
						Response: proto.FetchAuthChallengeResponseResponseProvideCredentials,
						Username: username,
						Password: password,
					},
				}.Call(b)
			}()
		},
	)

	go wait()

	return func() {
		_ = proto.FetchDisable{}.Call(b)
	}
}

// parseProxy extracts host:port and credentials from a proxy URL.
// Input:  "http://user:pass@host:port" → ("host:port", "user", "pass")
// Input:  "http://host:port"           → ("http://host:port", "", "")
// Input:  ""                           → ("", "", "")
func parseProxy(raw string) (server, user, pass string) {
	if raw == "" {
		return "", "", ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw, "", ""
	}
	pass, _ = u.User.Password()
	user = u.User.Username()
	// Reconstruct URL without credentials for Chrome's ProxyServer.
	u.User = nil
	return u.String(), user, pass
}

func (m *ChromeManager) getBrowser() *rod.Browser {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.browser
}

// reconnect closes the old connection and establishes a new one.
func (m *ChromeManager) reconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.browser != nil {
		_ = m.browser.Close()
		m.browser = nil
	}
	m.keepaliveCtxID = ""

	debuggerURL, err := discoverWSURL(m.wsURL)
	if err != nil {
		return fmt.Errorf("discover ws url: %w", err)
	}

	b := rod.New().ControlURL(debuggerURL)
	if err := b.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	keepaliveCtxID, err := createKeepaliveContext(b)
	if err != nil {
		_ = b.Close()
		return fmt.Errorf("keepalive context: %w", err)
	}

	m.browser = b
	m.keepaliveCtxID = keepaliveCtxID
	return nil
}

// NewStealthPage creates a page with stealth evasions applied.
// It runs go-rod/stealth JS patches followed by the complement JS that fills gaps
// not covered by CloakBrowser's C++ patches.
func (m *ChromeManager) NewStealthPage(ctx *rod.Browser, profile *StealthProfile) (*rod.Page, error) {
	page, err := stealth.Page(ctx)
	if err != nil {
		return nil, fmt.Errorf("chrome: stealth page: %w", err)
	}

	// Inject profile data before complement JS so modules can read __sp.
	if profile != nil {
		if _, err := page.EvalOnNewDocument(profile.InjectJS()); err != nil {
			_ = page.Close()
			return nil, fmt.Errorf("chrome: inject profile: %w", err)
		}
	}

	if _, err := page.EvalOnNewDocument(complementJS); err != nil {
		_ = page.Close()
		return nil, fmt.Errorf("chrome: eval complement js: %w", err)
	}

	// Set Accept-Language header to match profile languages so HTTP headers
	// and navigator.languages are consistent (detection scripts compare both).
	if profile != nil && len(profile.Langs) > 0 {
		langs := profile.Langs[0]
		for i, l := range profile.Langs[1:] {
			q := 0.9 - float64(i)*0.1
			if q < 0.1 {
				q = 0.1
			}
			langs += fmt.Sprintf(",%s;q=%.1f", l, q)
		}
		_ = proto.NetworkSetExtraHTTPHeaders{
			Headers: proto.NetworkHeaders{"Accept-Language": gson.New(langs)},
		}.Call(page)
	}

	return page, nil
}

// Connected reports whether the Chrome connection is active.
func (m *ChromeManager) Connected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.browser != nil
}

// Close disconnects from Chrome and releases resources.
func (m *ChromeManager) Close() {
	m.mu.Lock()
	b := m.browser
	m.browser = nil
	ctxID := m.keepaliveCtxID
	m.keepaliveCtxID = ""
	m.mu.Unlock()

	if b != nil {
		if ctxID != "" {
			_ = proto.TargetDisposeBrowserContext{BrowserContextID: ctxID}.Call(b)
		}
		_ = b.Close()
	}
}

// versionURL converts a ws:// or wss:// address to the http:///json/version endpoint.
func versionURL(wsURL string) string {
	u := wsURL
	u = strings.Replace(u, "wss://", "https://", 1)
	u = strings.Replace(u, "ws://", "http://", 1)
	return strings.TrimRight(u, "/") + "/json/version"
}

// chromeVersionResponse is a partial representation of the /json/version JSON response.
type chromeVersionResponse struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// discoverWSURL fetches /json/version and extracts the webSocketDebuggerUrl.
// Chrome rejects requests with non-IP Host headers, so we force Host: 127.0.0.1.
func discoverWSURL(wsURL string) (string, error) {
	vURL := versionURL(wsURL)

	req, err := http.NewRequest(http.MethodGet, vURL, nil) //nolint:noctx // no context in discovery helper
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	// Chrome's DevTools HTTP server rejects requests whose Host header is not an IP address.
	req.Host = "127.0.0.1"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get %s: %w", vURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var v chromeVersionResponse
	if err := json.Unmarshal(body, &v); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}

	if v.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("webSocketDebuggerUrl missing in /json/version response")
	}

	// The debugger URL often contains 127.0.0.1 (internal to Chrome container).
	// Replace the host:port with the original wsURL's host:port so it's reachable
	// from the go-browser container via Docker networking.
	return rewriteDebuggerURL(v.WebSocketDebuggerURL, wsURL), nil
}

// rewriteDebuggerURL replaces the host:port in a debugger URL with the original ws URL's host:port.
// Chrome returns ws://127.0.0.1/devtools/browser/GUID but we need ws://cloakbrowser:9222/devtools/browser/GUID.
func rewriteDebuggerURL(debuggerURL, originalWS string) string {
	// Extract path from debugger URL: /devtools/browser/GUID
	pathIdx := strings.Index(debuggerURL, "/devtools/")
	if pathIdx < 0 {
		return debuggerURL // can't parse, return as-is
	}
	path := debuggerURL[pathIdx:]

	// Extract host:port from original ws URL
	host := originalWS
	host = strings.TrimPrefix(host, "ws://")
	host = strings.TrimPrefix(host, "wss://")
	host = strings.TrimRight(host, "/")

	return "ws://" + host + path
}
