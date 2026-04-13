package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const (
	defaultTimeoutSecs = 30
	sessionIDNew       = "new"   // backward compat: session_id="new" → ephemeral named session
	modeDefault        = "default"
	modePrivate        = "private"
	modeProxy          = "proxy"
)

// InteractRequest is the JSON body for POST /chrome/interact.
type InteractRequest struct {
	URL         string   `json:"url"`
	Actions     []Action `json:"actions"`
	PreActions  []Action `json:"pre_actions,omitempty"` // executed after page creation, before navigation
	TimeoutSecs int      `json:"timeout_secs,omitempty"`
	Proxy       *string  `json:"proxy,omitempty"`
	// New session/mode params.
	Session string `json:"session,omitempty"` // named session; empty = ephemeral
	Mode    string `json:"mode,omitempty"`    // "default", "private" (default), "proxy"
	// Backward-compat params (still accepted, mapped to Session/Mode internally).
	SessionID  *string `json:"session_id,omitempty"`
	Profile    string  `json:"profile,omitempty"`
	UseProfile bool    `json:"use_profile,omitempty"` // → mode="default"
	ReusePage  bool    `json:"reuse_page,omitempty"`  // → session="__reuse__"
	NoStealth  bool    `json:"no_stealth,omitempty"`  // plain page without stealth JS injection
	StealthMode bool   `json:"stealth_mode,omitempty"` // route actions through cdputil
}

// InteractResponse is the JSON response for POST /chrome/interact.
type InteractResponse struct {
	URL       string         `json:"url"`
	Status    string         `json:"status"` // "ok" or "error"
	Actions   []ActionResult `json:"actions"`
	SessionID string         `json:"session_id,omitempty"`
	Error     string         `json:"error,omitempty"`
	ElapsedMs int64          `json:"elapsed_ms"`
}

func (s *Server) handleInteract(w http.ResponseWriter, r *http.Request) {
	if s.chrome == nil {
		writeError(w, http.StatusServiceUnavailable, "chrome not available")
		return
	}

	var req InteractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	timeoutSecs := req.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = defaultTimeoutSecs
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	proxy := ""
	if req.Proxy != nil {
		proxy = *req.Proxy
	}

	start := time.Now()
	resp := s.runInteract(ctx, req, proxy)
	resp.ElapsedMs = time.Since(start).Milliseconds()

	writeJSON(w, http.StatusOK, resp)
}

// RunInteract executes a Chrome interaction sequence using the ChromeManager's ContextPool.
func RunInteract(ctx context.Context, chrome *ChromeManager, req InteractRequest) InteractResponse {
	pool := chrome.Pool()
	if pool == nil {
		return InteractResponse{URL: req.URL, Status: "error", Error: "context pool not available"}
	}

	session, mode, proxy, ephemeral := resolveSessionParams(req)

	mp, err := pool.GetOrCreatePage(session, mode, proxy, req.URL)
	if err != nil {
		return InteractResponse{URL: req.URL, Status: "error", Error: err.Error()}
	}

	page := mp.Page
	isNewPage := page.MustInfo().URL == "about:blank" || page.MustInfo().URL == ""

	// Set up stealth / proxy auth on freshly created pages only.
	if isNewPage {
		if proxy != "" {
			_, proxyUser, proxyPass := parseProxy(proxy)
			if proxyUser != "" {
				cleanup := setupProxyAuth(page.Browser(), proxyUser, proxyPass)
				defer cleanup()
			}
		}

		if !req.NoStealth {
			profile, err := LoadProfile(req.Profile)
			if err != nil {
				return InteractResponse{URL: req.URL, Status: "error", Error: fmt.Sprintf("profile: %s", err)}
			}
			if err := applyStealthToExistingPage(page, profile); err != nil {
				return InteractResponse{URL: req.URL, Status: "error", Error: err.Error()}
			}
		}
	}

	if errMsg := runPreActions(ctx, page, req.PreActions); errMsg != "" {
		return InteractResponse{URL: req.URL, Status: "error", Error: errMsg}
	}

	// Navigate: skip if URL already matches or ReusePage is set.
	if req.URL != "" && req.URL != "about:blank" {
		if isNewPage || (!req.ReusePage && !strings.HasPrefix(page.MustInfo().URL, req.URL)) {
			if err := doNavigate(ctx, page, req.URL); err != nil {
				return InteractResponse{URL: req.URL, Status: "error", Error: err.Error()}
			}
		}
	}

	logs := NewLogCollector()
	logs.SubscribeCDP(page)

	cursor := humanize.NewCursor(390, 290)

	refMap := mp.Refs
	if refMap == nil {
		refMap = NewRefMap()
		mp.Refs = refMap
	}

	driftFunc := func(x, y float64) error {
		return proto.InputDispatchMouseEvent{
			Type: proto.InputDispatchMouseEventTypeMouseMoved,
			X:    x,
			Y:    y,
		}.Call(page)
	}
	stopDrift := humanize.StartIdleDrift(ctx, cursor, driftFunc)
	defer stopDrift()

	results := make([]ActionResult, 0, len(req.Actions))
	status := "ok"
	var actionErr string

	for _, a := range req.Actions {
		res := ExecuteAction(ctx, page, a, cursor, logs, req.StealthMode, refMap)
		results = append(results, res)
		if !res.Ok {
			status = "error"
			actionErr = res.Error
			break
		}
	}

	info, infoErr := page.Info()
	finalURL := req.URL
	if infoErr == nil {
		finalURL = info.URL
		mp.URL = finalURL
	}
	mp.LastUsed = time.Now()

	if ephemeral {
		_ = pool.ClosePage(session)
	}

	return InteractResponse{
		URL:       finalURL,
		Status:    status,
		Actions:   results,
		SessionID: session,
		Error:     actionErr,
	}
}

// resolveSessionParams maps old params (use_profile, session_id, proxy, reuse_page)
// to the new session/mode model, and determines if the session is ephemeral.
//
// Mapping rules:
//   - use_profile=true           → mode="default", session="__profile__", persistent
//   - proxy=<url>                → mode="proxy", session from session_id or auto
//   - session_id="new"           → mode="private", ephemeral=false (session auto-named)
//   - session_id=<id>            → mode="private", session=<id>, persistent
//   - reuse_page=true            → mode="default", session="__reuse__", persistent
//   - session=<name> (new param) → mode from Mode field, persistent
//   - nothing                    → mode="private", ephemeral
func resolveSessionParams(req InteractRequest) (session, mode, proxy string, ephemeral bool) {
	proxy = ""
	if req.Proxy != nil {
		proxy = *req.Proxy
	}

	// New-style params take priority.
	if req.Session != "" {
		session = req.Session
		mode = req.Mode
		if mode == "" {
			if proxy != "" {
				mode = modeProxy
			} else {
				mode = modePrivate
			}
		}
		return session, mode, proxy, false
	}

	// Backward compat: use_profile → default context.
	if req.UseProfile {
		return "__profile__", modeDefault, proxy, false
	}

	// Backward compat: reuse_page → reuse tab in default context.
	if req.ReusePage {
		return "__reuse__", modeDefault, proxy, false
	}

	// Backward compat: proxy without session → proxy mode, ephemeral.
	if proxy != "" && req.SessionID == nil {
		return generateEphemeralID(), modeProxy, proxy, true
	}

	// Backward compat: session_id="new" → create named session (auto-ID), persistent.
	if req.SessionID != nil && *req.SessionID == sessionIDNew {
		id := generateEphemeralID()
		mode = modePrivate
		if proxy != "" {
			mode = modeProxy
		}
		return id, mode, proxy, false
	}

	// Backward compat: session_id=<id> → reuse named session.
	if req.SessionID != nil && *req.SessionID != "" {
		mode = modePrivate
		if proxy != "" {
			mode = modeProxy
		}
		return *req.SessionID, mode, proxy, false
	}

	// No session — ephemeral.
	mode = modePrivate
	if proxy != "" {
		mode = modeProxy
	}
	return generateEphemeralID(), mode, proxy, true
}

// generateEphemeralID creates a short random ID for ephemeral sessions.
func generateEphemeralID() string {
	id, err := generateID()
	if err != nil {
		return fmt.Sprintf("eph-%d", time.Now().UnixNano())
	}
	return id
}

// runPreActions executes actions that must run before navigation (e.g. eval_on_new_document, set_cookies).
func runPreActions(ctx context.Context, page *rod.Page, actions []Action) string {
	for _, a := range actions {
		result := ExecuteAction(ctx, page, a, nil, nil, false, nil)
		if !result.Ok {
			return fmt.Sprintf("pre_action %s: %s", a.Type, result.Error)
		}
	}
	return ""
}

func (s *Server) runInteract(ctx context.Context, req InteractRequest, proxy string) InteractResponse {
	req.Proxy = &proxy
	return RunInteract(ctx, s.chrome, req)
}

func (s *Server) handleDestroySession(w http.ResponseWriter, r *http.Request) {
	if s.chrome == nil {
		writeError(w, http.StatusServiceUnavailable, "chrome not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	if err := s.chrome.Pool().ClosePage(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "destroyed", "id": id})
}
