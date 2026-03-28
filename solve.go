package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const (
	defaultSolveTimeoutSecs = 30
	cfClearanceCookie       = "cf_clearance"
	pollInterval            = 500 * time.Millisecond
)

// SolveRequest is the JSON body for POST /solve.
type SolveRequest struct {
	URL           string `json:"url"`
	ChallengeType string `json:"challenge_type,omitempty"`
	Proxy         string `json:"proxy,omitempty"`
	TimeoutSecs   int    `json:"timeout_secs,omitempty"`
}

// SolveResponse is the JSON response from POST /solve.
type SolveResponse struct {
	Status  string            `json:"status"`
	Cookies map[string]string `json:"cookies,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// SolveCF navigates to a URL via Chrome, waits for CF clearance cookie, returns all cookies.
func SolveCF(ctx context.Context, chrome *ChromeManager, url string, proxy string) (map[string]string, error) {
	scopedBrowser, ctxID, authCleanup, err := chrome.NewContext(proxy)
	if err != nil {
		return nil, fmt.Errorf("create browser context: %w", err)
	}
	if authCleanup != nil {
		defer authCleanup()
	}
	defer func() {
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: ctxID}.Call(scopedBrowser)
	}()

	page, err := chrome.NewStealthPage(scopedBrowser, nil)
	if err != nil {
		return nil, fmt.Errorf("create stealth page: %w", err)
	}
	defer func() { _ = page.Close() }()

	if err := page.Navigate(url); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	return waitForCFClearance(ctx, page, url)
}

// handleSolve navigates to a URL, waits for CF clearance cookie, and returns all cookies.
func (s *Server) handleSolve(w http.ResponseWriter, r *http.Request) {
	if s.chrome == nil {
		writeJSON(w, http.StatusServiceUnavailable, SolveResponse{
			Status: "error",
			Error:  "chrome not connected",
		})
		return
	}

	var req SolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SolveResponse{
			Status: "error",
			Error:  fmt.Sprintf("invalid request body: %s", err.Error()),
		})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, SolveResponse{
			Status: "error",
			Error:  "url is required",
		})
		return
	}

	timeoutSecs := req.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = defaultSolveTimeoutSecs
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cookies, err := SolveCF(ctx, s.chrome, req.URL, req.Proxy)
	if err != nil {
		writeJSON(w, http.StatusGatewayTimeout, SolveResponse{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, SolveResponse{
		Status:  "ok",
		Cookies: cookies,
	})
}

// waitForCFClearance polls page cookies every 500ms until cf_clearance is present or ctx expires.
func waitForCFClearance(ctx context.Context, page *rod.Page, _ string) (map[string]string, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for cf_clearance")
		case <-ticker.C:
			cookies, err := page.Cookies(nil)
			if err != nil {
				continue
			}

			result := make(map[string]string, len(cookies))
			for _, c := range cookies {
				result[c.Name] = c.Value
			}

			if _, ok := result[cfClearanceCookie]; ok {
				return result, nil
			}
		}
	}
}
