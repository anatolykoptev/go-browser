package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

const defaultRenderTimeoutSecs = 20

// RenderRequest is the JSON body for POST /render.
type RenderRequest struct {
	URL         string `json:"url"`
	TimeoutSecs int    `json:"timeout_secs,omitempty"`
	Proxy       string `json:"proxy,omitempty"`
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

// renderURL performs the actual Chrome navigation and HTML extraction.
func (s *Server) renderURL(req RenderRequest, timeout time.Duration) (*RenderResponse, error) {
	browser, contextID, authCleanup, err := s.chrome.NewContext(req.Proxy)
	if err != nil {
		return nil, fmt.Errorf("create context: %w", err)
	}
	if authCleanup != nil {
		defer authCleanup()
	}
	defer func() {
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: contextID}.Call(browser)
	}()

	page, err := s.chrome.NewStealthPage(browser, nil)
	if err != nil {
		return nil, fmt.Errorf("create stealth page: %w", err)
	}
	defer func() { _ = page.Close() }()

	page = page.Timeout(timeout)

	start := time.Now()

	if err := page.Navigate(req.URL); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNavigate, err)
	}

	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait load: %w", err)
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
