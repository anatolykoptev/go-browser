// Package cdpmock provides a mock Chrome DevTools Protocol server for testing
// recovery paths without a real Chrome instance. It speaks the CDP protocol
// (JSON over WebSocket) well enough for rod.Browser to connect, and supports
// fault injection to simulate Chrome crashes, hangs, and errors.
//
// #54: Enables deterministic testing of reconnect, stale context recovery,
// placeholder timeout, and LostConnection detection in CI without provisioning
// Chromium.
package cdpmock

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Server is a mock CDP server that listens on a random port and speaks the
// Chrome DevTools Protocol over WebSocket. It serves /json/version for
// discovery and handles CDP commands on the WebSocket connection.
//
// The WebSocket implementation is rod-compatible (raw TCP + manual handshake)
// to avoid gorilla/websocket handshake validation issues with rod's non-standard
// Sec-WebSocket-Key.
type Server struct {
	listener   net.Listener
	wsURL      string

	// Fault injection controls (all atomic / mutex-protected for concurrent access).
	closeOnNext atomic.Bool   // close WebSocket on next message
	delayMs     atomic.Int64  // delay before responding (ms)
	malformed   atomic.Bool   // send garbage instead of valid JSON
	mu          sync.Mutex
	errorMap    map[string]error // method → error to return

	// CDP state
	nextTargetID int
	targets      map[string]TargetInfo

	// Connection management
	connMu sync.Mutex
	conn   net.Conn // current WebSocket connection

	closed chan struct{}
}

// TargetInfo is a minimal CDP target description.
type TargetInfo struct {
	TargetID         string `json:"targetId"`
	Type             string `json:"type"`
	URL              string `json:"url"`
	Title            string `json:"title"`
	BrowserContextID string `json:"browserContextId,omitempty"`
}

// cdpRequest is the JSON shape of a CDP command request.
type cdpRequest struct {
	ID        int             `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// cdpResponse is the JSON shape of a CDP command response.
type cdpResponse struct {
	ID        int             `json:"id"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// cdpError is the CDP error shape.
type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// cdpEvent is the JSON shape of a CDP event (server → client).
type cdpEvent struct {
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// New creates a new mock CDP server. Call Start to bind to a port.
func New() *Server {
	return &Server{
		errorMap: make(map[string]error),
		targets:  make(map[string]TargetInfo),
		closed:   make(chan struct{}),
	}
}

// Start binds the server to a random localhost port and begins serving.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("cdpmock: listen: %w", err)
	}
	s.listener = ln
	port := ln.Addr().(*net.TCPAddr).Port
	s.wsURL = fmt.Sprintf("ws://127.0.0.1:%d", port)

	go s.serve()
	return nil
}

func (s *Server) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
			}
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// Read the HTTP request.
	r := bufio.NewReader(conn)
	req, err := http.ReadRequest(r)
	if err != nil {
		return
	}

	// Route based on path.
	switch req.URL.Path {
	case "/json/version":
		s.handleVersionHTTP(conn, req)
		return
	default:
		// WebSocket upgrade.
		s.handleWebSocketUpgrade(conn, r, req)
		return
	}
}

// WSURL returns the WebSocket URL for rod.New().ControlURL().
func (s *Server) WSURL() string { return s.wsURL }

// Close shuts down the mock server.
func (s *Server) Close() {
	select {
	case <-s.closed:
		return
	default:
	}
	close(s.closed)
	_ = s.listener.Close()
}

// --- Fault injection ---

// InjectClose closes the WebSocket connection on the next message,
// simulating a Chrome crash or network drop.
func (s *Server) InjectClose() {
	s.closeOnNext.Store(true)
}

// InjectDelay adds a delay before each CDP response, simulating a slow
// or hung Chrome. Set to 0 to disable.
func (s *Server) InjectDelay(d time.Duration) {
	s.delayMs.Store(d.Milliseconds())
}

// InjectError configures the server to return a CDP error for a specific
// method. Pass nil error to clear.
func (s *Server) InjectError(method string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.errorMap, method)
	} else {
		s.errorMap[method] = err
	}
}

// InjectMalformed makes the server send garbage instead of valid JSON,
// simulating a corrupted CDP stream.
func (s *Server) InjectMalformed() {
	s.malformed.Store(true)
}

// --- HTTP handlers ---

func (s *Server) handleVersionHTTP(conn net.Conn, req *http.Request) {
	body, _ := json.Marshal(map[string]any{
		"webSocketDebuggerUrl":  s.wsURL,
		"Browser":               "cdpmock/v1.0",
		"Protocol-Version":      "1.3",
		"User-Agent":            "cdpmock",
		"V8-Version":            "mock",
		"WebKit-Version":        "mock",
	})
	resp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
	_, _ = conn.Write([]byte(resp))
}

func (s *Server) handleWebSocketUpgrade(conn net.Conn, r *bufio.Reader, req *http.Request) {
	// Compute Sec-WebSocket-Accept.
	key := req.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		key = "nil"
	}
	accept := computeAccept(key)

	resp := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	_, err := conn.Write([]byte(resp))
	if err != nil {
		return
	}

	// Now we have a WebSocket connection. Store it.
	s.connMu.Lock()
	s.conn = conn
	s.connMu.Unlock()

	defer func() {
		s.connMu.Lock()
		s.conn = nil
		s.connMu.Unlock()
	}()

	s.serveCDP(conn, r)
}

func computeAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// --- WebSocket frame handling (server-side: no masking) ---

func wsWrite(conn net.Conn, data []byte) error {
	// Server-to-client frames are NOT masked.
	// FIN=1, opcode=1 (text)
	header := []byte{0b1000_0001}

	size := len(data)
	switch {
	case size <= 125:
		header = append(header, byte(size))
	case size < 65536:
		header = append(header, 126, byte(size>>8), byte(size))
	default:
		header = append(header, 127)
		// 8-byte length
		for i := 7; i >= 0; i-- {
			header = append(header, byte(size>>(uint(i)*8)))
		}
	}

	_, err := conn.Write(append(header, data...))
	return err
}

func wsRead(r *bufio.Reader) ([]byte, error) {
	// Read first byte (FIN + opcode)
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	_ = b // FIN and opcode — we don't care for client frames

	// Read second byte (mask + payload length)
	b, err = r.ReadByte()
	if err != nil {
		return nil, err
	}
	masked := b&0b1000_0000 != 0
	size := int(b & 0x7f)

	switch size {
	case 126:
		b1, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		b2, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		size = int(b1)<<8 + int(b2)
	case 127:
		size = 0
		for i := 0; i < 8; i++ {
			b, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			size = size<<8 + int(b)
		}
	}

	// Read mask key if present (client frames MUST be masked)
	var mask [4]byte
	if masked {
		for i := 0; i < 4; i++ {
			b, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			mask[i] = b
		}
	}

	// Read payload
	data := make([]byte, size)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return nil, err
	}

	// Unmask
	if masked {
		for i := range data {
			data[i] ^= mask[i%4]
		}
	}

	return data, nil
}

// --- CDP command loop ---

func (s *Server) serveCDP(conn net.Conn, r *bufio.Reader) {
	for {
		data, err := wsRead(r)
		if err != nil {
			return // connection closed
		}

		// Check fault injection: close on next message.
		if s.closeOnNext.Load() {
			_ = conn.Close()
			return
		}

		var req cdpRequest
		if err := json.Unmarshal(data, &req); err != nil {
			continue // ignore malformed requests
		}

		// Apply delay if configured.
		if d := s.delayMs.Load(); d > 0 {
			time.Sleep(time.Duration(d) * time.Millisecond)
		}

		// Check for injected error.
		s.mu.Lock()
		injErr, hasErr := s.errorMap[req.Method]
		s.mu.Unlock()

		if hasErr {
			s.sendCDPResponse(conn, cdpResponse{
				ID: req.ID,
				Error: &cdpError{
					Code:    -32000,
					Message: injErr.Error(),
				},
			})
			continue
		}

		// Handle the CDP command.
		result, err := s.handleCommand(req.Method, req.Params)
		if err != nil {
			s.sendCDPResponse(conn, cdpResponse{
				ID: req.ID,
				Error: &cdpError{
					Code:    -32601,
					Message: err.Error(),
				},
			})
			continue
		}

		resultJSON, _ := json.Marshal(result)
		s.sendCDPResponse(conn, cdpResponse{
			ID:     req.ID,
			Result: resultJSON,
		})
	}
}

func (s *Server) sendCDPResponse(conn net.Conn, resp cdpResponse) {
	if s.malformed.Load() {
		_ = wsWrite(conn, []byte("{{{garbage}}}"))
		return
	}
	data, _ := json.Marshal(resp)
	_ = wsWrite(conn, data)
}

// SendEvent sends a CDP event to the connected client (simulating Chrome
// sending events like Target.targetCreated, Page.frameNavigated, etc.).
func (s *Server) SendEvent(method string, params any) {
	s.connMu.Lock()
	conn := s.conn
	s.connMu.Unlock()
	if conn == nil {
		return
	}
	paramsJSON, _ := json.Marshal(params)
	data, _ := json.Marshal(cdpEvent{
		Method: method,
		Params: paramsJSON,
	})
	_ = wsWrite(conn, data)
}

// CloseConnection forcibly closes the WebSocket connection, simulating
// a Chrome crash or network drop.
func (s *Server) CloseConnection() {
	s.connMu.Lock()
	conn := s.conn
	s.connMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// --- CDP command handlers ---

func (s *Server) handleCommand(method string, params json.RawMessage) (any, error) {
	switch method {
	case "Browser.getVersion":
		return map[string]any{
			"protocolVersion": "1.3",
			"product":         "cdpmock/v1.0",
			"revision":        "mock",
			"userAgent":       "cdpmock",
			"jsVersion":       "mock",
		}, nil

	case "Target.getTargets":
		var targets []TargetInfo
		s.mu.Lock()
		for _, t := range s.targets {
			targets = append(targets, t)
		}
		s.mu.Unlock()
		return map[string]any{"targetInfos": targets}, nil

	case "Target.createTarget":
		var p struct {
			URL              string `json:"url"`
			BrowserContextID string `json:"browserContextId,omitempty"`
		}
		_ = json.Unmarshal(params, &p)
		s.mu.Lock()
		s.nextTargetID++
		id := fmt.Sprintf("target-%d", s.nextTargetID)
		t := TargetInfo{
			TargetID:         id,
			Type:             "page",
			URL:              p.URL,
			BrowserContextID: p.BrowserContextID,
		}
		s.targets[id] = t
		s.mu.Unlock()
		return map[string]any{"targetId": id}, nil

	case "Target.closeTarget":
		var p struct {
			TargetID string `json:"targetId"`
		}
		_ = json.Unmarshal(params, &p)
		s.mu.Lock()
		delete(s.targets, p.TargetID)
		s.mu.Unlock()
		return map[string]any{}, nil

	case "Target.createBrowserContext":
		s.mu.Lock()
		s.nextTargetID++
		ctxID := fmt.Sprintf("ctx-%d", s.nextTargetID)
		s.mu.Unlock()
		return map[string]any{"browserContextId": ctxID}, nil

	case "Target.disposeBrowserContext":
		return map[string]any{}, nil

	case "Target.attachToTarget":
		var p struct {
			TargetID string `json:"targetId"`
			Flatten  bool   `json:"flatten"`
		}
		_ = json.Unmarshal(params, &p)
		s.mu.Lock()
		s.nextTargetID++
		sessID := fmt.Sprintf("sess-%d", s.nextTargetID)
		s.mu.Unlock()
		return map[string]any{"sessionId": sessID}, nil

	case "Target.detachFromTarget":
		return map[string]any{}, nil

	case "Target.setDiscoverTargets":
		return map[string]any{}, nil

	// Fetch domain (egress guard) — accept but do nothing.
	case "Fetch.enable":
		return map[string]any{}, nil
	case "Fetch.disable":
		return map[string]any{}, nil
	case "Fetch.continueRequest":
		return map[string]any{}, nil
	case "Fetch.failRequest":
		return map[string]any{}, nil
	case "Fetch.continueWithAuth":
		return map[string]any{}, nil

	// Network domain — return -32601 to simulate CloakBrowser (no Network support).
	case "Network.enable":
		return nil, fmt.Errorf("'Network.enable' wasn't found")

	// Page domain — accept.
	case "Page.enable":
		return map[string]any{}, nil
	case "Page.disable":
		return map[string]any{}, nil
	case "Page.navigate":
		var p struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(params, &p)
		return map[string]any{
			"frameId":  "frame-1",
			"loaderId": "loader-1",
		}, nil
	case "Page.close":
		return map[string]any{}, nil
	case "Page.getFrameTree":
		return map[string]any{
			"frameTree": map[string]any{
				"frame": map[string]any{
					"id":   "frame-1",
					"url":  "about:blank",
					"name": "",
				},
			},
		}, nil
	case "Page.setLifecycleEventsEnabled":
		return map[string]any{}, nil

	// Runtime domain — accept.
	case "Runtime.enable":
		return map[string]any{}, nil
	case "Runtime.disable":
		return map[string]any{}, nil
	case "Runtime.evaluate":
		return map[string]any{
			"result": map[string]any{
				"type":  "string",
				"value": "cdpmock-result",
			},
		}, nil
	case "Runtime.addBinding":
		return map[string]any{}, nil

	// DOM domain — accept.
	case "DOM.getDocument":
		return map[string]any{
			"root": map[string]any{
				"nodeId":        1,
				"backendNodeId": 1,
				"nodeType":      9,
				"nodeName":      "#document",
				"childNodeCount": 0,
			},
		}, nil
	case "DOM.describeNode":
		return map[string]any{
			"node": map[string]any{
				"nodeId": 1,
			},
		}, nil

	// Console domain — accept.
	case "Console.enable":
		return map[string]any{}, nil

	// Log domain — accept.
	case "Log.enable":
		return map[string]any{}, nil
	case "Log.clear":
		return map[string]any{}, nil

	// Emulation domain — accept (stealth overrides).
	case "Emulation.setTimezoneOverride":
		return map[string]any{}, nil
	case "Emulation.setLocaleOverride":
		return map[string]any{}, nil
	case "Emulation.setUserAgentOverride":
		return map[string]any{}, nil
	case "Network.setExtraHTTPHeaders":
		return map[string]any{}, nil

	// Security domain — accept.
	case "Security.setIgnoreCertificateErrors":
		return map[string]any{}, nil

	default:
		// Unknown method — return minimal success to avoid breaking rod.
		slog.Debug("cdpmock: unhandled method", "method", method)
		return map[string]any{}, nil
	}
}
