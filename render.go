package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const defaultRenderTimeoutSecs = 20

// networkIdleQuiet is the duration with zero in-flight requests before
// WaitRequestIdle declares the page "idle". 500ms matches Puppeteer's
// networkidle0/2 semantics and is enough for Google CSE / analytics
// scripts to finish without waiting indefinitely on long-poll connections.
const networkIdleQuiet = 500 * time.Millisecond

// RenderRequest is the JSON body for POST /render.
type RenderRequest struct {
	URL         string `json:"url"`
	TimeoutSecs int    `json:"timeout_secs,omitempty"`
	Proxy       string `json:"proxy,omitempty"`
	Wait        string `json:"wait,omitempty"` // wait strategy: "load" (default), "domcontentloaded", "networkidle"
}

// RenderResponse is the JSON body returned by POST /render.
type RenderResponse struct {
	URL       string `json:"url"`
	HTML      string `json:"html"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	ElapsedMs int64  `json:"elapsed_ms"`
	Error     string `json:"error,omitempty"`
}

// handleRender navigates to a URL with a stealth Chrome page and returns the rendered HTML.
func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	if s.chrome == nil || !s.chrome.Connected() {
		writeError(w, http.StatusServiceUnavailable, "chrome not connected")
		return
	}

	var req RenderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid json: %s", err))
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	timeoutSecs := req.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = defaultRenderTimeoutSecs
	}

	resp, err := s.renderURL(req, time.Duration(timeoutSecs)*time.Second)
	if err != nil {
		writeJSON(w, http.StatusOK, RenderResponse{
			URL:   req.URL,
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// RenderURL renders a URL in a stealth Chrome page and returns HTML.
//
// The wait strategy is controlled by req.Wait:
//   - "load" (default): waits for window.onload — matches historical behavior.
//   - "domcontentloaded": waits for DOMContentLoaded only — faster, but JS
//     may not have finished mutating the DOM.
//   - "networkidle": waits for onload AND then for network to go idle
//     (no in-flight requests for networkIdleQuiet). Use for JS-heavy pages
//     that render content asynchronously (Google CSE, React/Vue apps).
func RenderURL(chrome *ChromeManager, req RenderRequest, timeout time.Duration) (*RenderResponse, error) {
	browser, contextID, authCleanup, err := chrome.NewContext(req.Proxy)
	if err != nil {
		return nil, fmt.Errorf("create context: %w", err)
	}
	if authCleanup != nil {
		defer authCleanup()
	}
	defer func() {
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: contextID}.Call(browser)
	}()

	page, err := chrome.NewStealthPage(browser, nil)
	if err != nil {
		return nil, fmt.Errorf("create stealth page: %w", err)
	}
	defer func() { _ = page.Close() }()

	page = page.Timeout(timeout)

	start := time.Now()

	if err := page.Navigate(req.URL); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNavigate, err)
	}

	if err := waitPageReady(page, req.Wait, timeout, start); err != nil {
		return nil, err
	}

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("get html: %w", err)
	}

	info, err := page.Info()
	if err != nil {
		return nil, fmt.Errorf("get page info: %w", err)
	}

	return &RenderResponse{
		URL:       info.URL,
		HTML:      html,
		Title:     info.Title,
		Status:    http.StatusOK,
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}

// waitPageReady blocks until the page reaches the requested readiness state.
// Unknown or empty wait defaults to "load" (historical behavior).
//
// For "networkidle", the idle wait is bounded to 80% of the total timeout
// (measured from start) so that HTML extraction always has at least 20% of
// the budget left. If the network doesn't go idle within that window, we
// proceed with whatever has loaded so far — a partial render is better than
// a timeout. The idle wait runs on a child context (page.Timeout) so its
// cancellation does not affect the parent page's ability to extract HTML.
func waitPageReady(page *rod.Page, wait string, totalTimeout time.Duration, start time.Time) error {
	switch strings.ToLower(strings.TrimSpace(wait)) {
	case "domcontentloaded":
		waitFn := page.WaitNavigation(proto.PageLifecycleEventNameDOMContentLoaded)
		waitFn()
		return nil
	case "networkidle":
		// WaitLoad first — networkidle without onload can fire prematurely
		// on pages with deferred scripts that haven't started yet.
		if err := page.WaitLoad(); err != nil {
			return fmt.Errorf("wait load: %w", err)
		}
		// Bound the idle wait to 80% of the total timeout from start, leaving
		// at least 20% for HTML extraction. The child page.Timeout context
		// isolates the idle wait — if it expires, the parent page is unaffected
		// and HTML() can still run.
		idleBudget := totalTimeout*4/5 - time.Since(start)
		if idleBudget > networkIdleQuiet {
			idlePage := page.Timeout(idleBudget)
			waitFn := idlePage.WaitRequestIdle(networkIdleQuiet, nil, nil, nil)
			waitFn()
		}
		return nil
	default: // "load" or empty
		if err := page.WaitLoad(); err != nil {
			return fmt.Errorf("wait load: %w", err)
		}
		return nil
	}
}

// renderURL performs the actual Chrome navigation and HTML extraction.
func (s *Server) renderURL(req RenderRequest, timeout time.Duration) (*RenderResponse, error) {
	return RenderURL(s.chrome, req, timeout)
}
