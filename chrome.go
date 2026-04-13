package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

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

	// Clean up orphaned contexts from previous go-wowa restarts.
	// These are incognito contexts with only about:blank pages (stale keepalives).
	cleanOrphanedContexts(b)

	// Ensure at least one default-context page exists; close extras.
	ensureDefaultPage(b)

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
// Retained for callers that need a hard reset; normal startup uses ensureDefaultPage.
//
//nolint:unused
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

// ensureDefaultPage makes sure at least one page exists in the default context.
// If none exist, creates one at about:blank.
func ensureDefaultPage(b *rod.Browser) {
	targets, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		return
	}

	var defaultPages []proto.TargetTargetInfo
	for _, t := range targets.TargetInfos {
		if t.Type == "page" && t.BrowserContextID == "" {
			defaultPages = append(defaultPages, *t)
		}
	}

	if len(defaultPages) == 0 {
		// No default pages — create one.
		_, _ = proto.TargetCreateTarget{URL: "about:blank"}.Call(b)
		return
	}

	// Close extra default pages (keep the first one).
	for i := 1; i < len(defaultPages); i++ {
		_, _ = proto.TargetCloseTarget{TargetID: defaultPages[i].TargetID}.Call(b)
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

// cleanOrphanedContexts removes incognito browser contexts that contain only
// about:blank pages — leftovers from previous go-wowa keepalive contexts.
// Contexts with real pages (non-blank URLs) are preserved.
func cleanOrphanedContexts(b *rod.Browser) {
	targets, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		return
	}

	// Group pages by context. Track which contexts have real (non-blank) pages.
	type ctxInfo struct {
		pageIDs []proto.TargetTargetID
		hasReal bool
	}
	contexts := make(map[proto.BrowserBrowserContextID]*ctxInfo)

	for _, t := range targets.TargetInfos {
		if t.Type != "page" || t.BrowserContextID == "" {
			continue // skip default context and non-page targets
		}
		ci, ok := contexts[t.BrowserContextID]
		if !ok {
			ci = &ctxInfo{}
			contexts[t.BrowserContextID] = ci
		}
		ci.pageIDs = append(ci.pageIDs, t.TargetID)
		if t.URL != "" && t.URL != "about:blank" && t.URL != "chrome://newtab/" {
			ci.hasReal = true
		}
	}

	// Dispose contexts that only have blank pages.
	for ctxID, ci := range contexts {
		if ci.hasReal {
			continue
		}
		for _, pid := range ci.pageIDs {
			_, _ = proto.TargetCloseTarget{TargetID: pid}.Call(b)
		}
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: ctxID}.Call(b)
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
