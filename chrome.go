package browser

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

//go:embed stealth_complement.js
var complementJS string

// ChromeManager manages a connection to a remote Chrome instance (CloakBrowser).
type ChromeManager struct {
	mu      sync.RWMutex
	browser *rod.Browser
	wsURL   string // original ws URL for discovery (e.g. ws://cloakbrowser:9222)
}

// NewChromeManager connects to a remote Chrome via WebSocket debugger URL.
// wsURL may be a ws:// address; the actual debugger URL is discovered via /json/version.
func NewChromeManager(wsURL string) (*ChromeManager, error) {
	debuggerURL, err := discoverWSURL(wsURL)
	if err != nil {
		return nil, fmt.Errorf("chrome: discover ws url: %w", err)
	}

	b := rod.New().ControlURL(debuggerURL)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("chrome: connect: %w", err)
	}

	return &ChromeManager{
		browser: b,
		wsURL:   wsURL,
	}, nil
}

// NewContext creates an isolated BrowserContext with optional proxy.
// Returns a browser scoped to that context and the context ID for lifecycle management.
// Automatically reconnects once if the CDP connection is dead.
func (m *ChromeManager) NewContext(proxy string) (*rod.Browser, proto.BrowserBrowserContextID, error) {
	b := m.getBrowser()
	if b == nil {
		return nil, "", ErrUnavailable
	}

	res, err := proto.TargetCreateBrowserContext{
		ProxyServer:     proxy,
		DisposeOnDetach: true,
	}.Call(b)
	if err != nil {
		// Attempt reconnect once on connection error.
		if reconnErr := m.reconnect(); reconnErr != nil {
			return nil, "", fmt.Errorf("chrome: create browser context: %w (reconnect failed: %v)", err, reconnErr)
		}
		b = m.getBrowser()
		if b == nil {
			return nil, "", fmt.Errorf("chrome: reconnect succeeded but browser is nil")
		}
		res, err = proto.TargetCreateBrowserContext{
			ProxyServer:     proxy,
			DisposeOnDetach: true,
		}.Call(b)
		if err != nil {
			return nil, "", fmt.Errorf("chrome: create browser context after reconnect: %w", err)
		}
	}

	scoped := b.NoDefaultDevice()
	scoped.BrowserContextID = res.BrowserContextID

	return scoped, res.BrowserContextID, nil
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

	debuggerURL, err := discoverWSURL(m.wsURL)
	if err != nil {
		return fmt.Errorf("discover ws url: %w", err)
	}

	b := rod.New().ControlURL(debuggerURL)
	if err := b.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	m.browser = b
	return nil
}

// NewStealthPage creates a page with stealth evasions applied.
// It runs go-rod/stealth JS patches followed by the complement JS that fills gaps
// not covered by CloakBrowser's C++ patches.
func (m *ChromeManager) NewStealthPage(ctx *rod.Browser) (*rod.Page, error) {
	page, err := stealth.Page(ctx)
	if err != nil {
		return nil, fmt.Errorf("chrome: stealth page: %w", err)
	}

	if _, err := page.EvalOnNewDocument(complementJS); err != nil {
		_ = page.Close()
		return nil, fmt.Errorf("chrome: eval complement js: %w", err)
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
	m.mu.Unlock()

	if b != nil {
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
