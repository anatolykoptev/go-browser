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
	keepaliveCtxID proto.BrowserBrowserContextID // unused, kept for lifecycle compat
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

	// Clean up orphaned contexts from previous restarts (blank-only incognito windows).
	cleanOrphanedContexts(b)

	// Don't create any pages — Chrome with Xvfb stays alive on its own.
	// If user needs a page, chrome_interact will create one on demand.

	return &ChromeManager{
		browser: b,
		wsURL:   wsURL,
	}, nil
}

// createKeepaliveContext creates a browser context with a blank page that prevents
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
