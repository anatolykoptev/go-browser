package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod/lib/proto"
)

const (
	defaultTimeoutSecs = 30
	sessionIDNew       = "new"
)

// InteractRequest is the JSON body for POST /chrome/interact.
type InteractRequest struct {
	URL         string   `json:"url"`
	Actions     []Action `json:"actions"`
	TimeoutSecs int      `json:"timeout_secs,omitempty"`
	Proxy       *string  `json:"proxy,omitempty"`
	SessionID   *string  `json:"session_id,omitempty"`
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

func (s *Server) runInteract(ctx context.Context, req InteractRequest, proxy string) InteractResponse {
	wantSession := req.SessionID != nil && *req.SessionID == sessionIDNew

	browser, contextID, err := s.chrome.NewContext(proxy)
	if err != nil {
		return InteractResponse{URL: req.URL, Status: "error", Error: err.Error()}
	}

	// Dispose context when done, unless we're persisting as a session.
	// Session persistence is tracked via wantSession; dispose runs regardless
	// if session creation fails below.
	disposeCtx := true
	defer func() {
		if disposeCtx {
			_ = proto.TargetDisposeBrowserContext{BrowserContextID: contextID}.Call(browser)
		}
	}()

	page, err := s.chrome.NewStealthPage(browser)
	if err != nil {
		return InteractResponse{URL: req.URL, Status: "error", Error: err.Error()}
	}
	defer func() { _ = page.Close() }()

	logs := NewLogCollector()
	logs.SubscribeCDP(page)

	if err := page.Context(ctx).Navigate(req.URL); err != nil {
		return InteractResponse{URL: req.URL, Status: "error", Error: "navigate: " + err.Error()}
	}
	if err := page.Context(ctx).WaitLoad(); err != nil {
		return InteractResponse{URL: req.URL, Status: "error", Error: "wait_load: " + err.Error()}
	}

	cursor := humanize.NewCursor(390, 290)

	// Start idle drift
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
		res := ExecuteAction(ctx, page, a, cursor, logs)
		results = append(results, res)
		if !res.Ok {
			status = "error"
			actionErr = res.Error
			break
		}
	}

	info, err := page.Info()
	finalURL := req.URL
	if err == nil {
		finalURL = info.URL
	}

	var sessionID string
	if wantSession && s.pool != nil {
		id, err := s.pool.Create(proxy)
		if err == nil {
			sessionID = id
			disposeCtx = false // pool owns the context now
		}
	}

	return InteractResponse{
		URL:       finalURL,
		Status:    status,
		Actions:   results,
		SessionID: sessionID,
		Error:     actionErr,
	}
}

func (s *Server) handleDestroySession(w http.ResponseWriter, r *http.Request) {
	if s.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "session pool not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	if !s.pool.Destroy(id) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "destroyed", "id": id})
}
