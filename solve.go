package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	Status    string            `json:"status"`
	Cookies   map[string]string `json:"cookies,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`
	Body      string            `json:"body,omitempty"`
	FinalURL  string            `json:"final_url,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// SolveResult is what a cleared challenge session produced — the same
// contract FlareSolverr's `solution` object carries (cookies + userAgent +
// response HTML). Callers that only need the page can serve Body directly
// instead of replaying cookies through a different client fingerprint,
// which CF binds to the solving session and rejects on mismatch
// (ox-browser#162).
type SolveResult struct {
	Cookies   map[string]string
	UserAgent string // navigator.userAgent of the solving session
	Body      string // post-clearance page HTML; empty while still challenged
	FinalURL  string // page URL after clearance (CF may redirect)
}

// SolveCF navigates to a URL via Chrome, waits for CF clearance cookie, and
// returns the session cookies plus the solving browser's UA and the page it
// landed on.
func SolveCF(ctx context.Context, chrome *ChromeManager, url string, proxy string) (*SolveResult, error) {
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

	cookies, err := waitForCFClearance(ctx, page, url)
	if err != nil {
		return nil, err
	}

	res := &SolveResult{Cookies: cookies}
	// UA the clearance was bound to — lets a resend carry the same UA even
	// when the body path isn't taken (non-idempotent requests).
	if ua, err := page.Eval(`() => navigator.userAgent`); err == nil {
		res.UserAgent = ua.Value.Str()
	}
	if info, err := page.Info(); err == nil {
		res.FinalURL = info.URL
	}
	res.Body = waitForSettledHTML(ctx, page)
	return res, nil
}

// challengeMarkers are strings present in CF's interstitial that must be
// gone before the page HTML is safe to serve as solved content.
var challengeMarkers = []string{
	"Just a moment",         // challenge page <title>
	"challenge-platform",    // CF challenge script container
	"cf-chl-",               // legacy challenge element ids
	"Checking your browser", // interstitial copy
	"window._cf_chl_opt",    // challenge bootstrap object
}

// looksLikeChallenge reports whether an HTML snapshot is still the CF
// interstitial rather than origin content.
func looksLikeChallenge(html string) bool {
	for _, m := range challengeMarkers {
		if strings.Contains(html, m) {
			return true
		}
	}
	return false
}

// waitForSettledHTML polls page.HTML() until the DOM no longer looks like a
// CF challenge or ctx expires. Returns "" when the deadline hits while still
// challenged — an empty Body sends the caller down the cookie-resend path,
// which is honest, versus serving the interstitial as content.
func waitForSettledHTML(ctx context.Context, page *rod.Page) string {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		html, err := page.HTML()
		if err == nil && !looksLikeChallenge(html) && html != "" {
			return html
		}
		select {
		case <-ctx.Done():
			return ""
		case <-ticker.C:
		}
	}
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

	res, err := SolveCF(ctx, s.chrome, req.URL, req.Proxy)
	if err != nil {
		writeJSON(w, http.StatusGatewayTimeout, SolveResponse{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, SolveResponse{
		Status:    "ok",
		Cookies:   res.Cookies,
		UserAgent: res.UserAgent,
		Body:      res.Body,
		FinalURL:  res.FinalURL,
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
