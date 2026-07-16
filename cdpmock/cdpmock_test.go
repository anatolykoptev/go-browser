package cdpmock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// TestMockServer_BasicConnect verifies that rod can connect to the mock
// and Browser.getVersion works.
func TestMockServer_BasicConnect(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Close()

	b := rod.New().ControlURL(s.WSURL())
	if err := b.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() { _ = b.Close() }()

	ver, err := (&proto.BrowserGetVersion{}).Call(b)
	if err != nil {
		t.Fatalf("BrowserGetVersion failed: %v", err)
	}

	if ver.Product != "cdpmock/v1.0" {
		t.Fatalf("expected cdpmock/v1.0, got %s", ver.Product)
	}

	t.Logf("Version: %s, Protocol: %s", ver.Product, ver.ProtocolVersion)
}

// TestMockServer_TargetCreate verifies Target.createTarget works.
func TestMockServer_TargetCreate(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Close()

	b := rod.New().ControlURL(s.WSURL())
	if err := b.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() { _ = b.Close() }()

	res, err := proto.TargetCreateTarget{URL: "about:blank"}.Call(b)
	if err != nil {
		t.Fatalf("TargetCreateTarget failed: %v", err)
	}

	if res.TargetID == "" {
		t.Fatal("expected non-empty targetId")
	}

	t.Logf("Created target: %s", res.TargetID)
}

// TestMockServer_InjectError verifies that InjectError causes a CDP error.
func TestMockServer_InjectError(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Close()

	s.InjectError("Browser.getVersion", fmt.Errorf("mock error: browser unavailable"))

	b := rod.New().ControlURL(s.WSURL())
	if err := b.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() { _ = b.Close() }()

	_, err := (&proto.BrowserGetVersion{}).Call(b)
	if err == nil {
		t.Fatal("expected error from BrowserGetVersion, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

// TestMockServer_InjectClose verifies that InjectClose drops the connection.
func TestMockServer_InjectClose(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Close()

	b := rod.New().ControlURL(s.WSURL())
	if err := b.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() { _ = b.Close() }()

	// First call succeeds.
	_, err := (&proto.BrowserGetVersion{}).Call(b)
	if err != nil {
		t.Fatalf("first BrowserGetVersion failed: %v", err)
	}

	// Inject close — next call should fail.
	s.InjectClose()

	_, err = (&proto.BrowserGetVersion{}).Call(b)
	if err == nil {
		t.Fatal("expected error after InjectClose, got nil")
	}

	t.Logf("Got expected error after close: %v", err)
}

// TestMockServer_InjectDelay verifies that InjectDelay slows down responses.
func TestMockServer_InjectDelay(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Close()

	b := rod.New().ControlURL(s.WSURL())
	if err := b.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() { _ = b.Close() }()

	// Baseline: fast response.
	start := time.Now()
	_, err := (&proto.BrowserGetVersion{}).Call(b)
	if err != nil {
		t.Fatalf("first BrowserGetVersion failed: %v", err)
	}
	baseline := time.Since(start)

	// Inject 200ms delay.
	s.InjectDelay(200 * time.Millisecond)
	defer s.InjectDelay(0)

	start = time.Now()
	_, err = (&proto.BrowserGetVersion{}).Call(b)
	if err != nil {
		t.Fatalf("delayed BrowserGetVersion failed: %v", err)
	}
	delayed := time.Since(start)

	if delayed < 150*time.Millisecond {
		t.Fatalf("expected ~200ms delay, got %v (baseline %v)", delayed, baseline)
	}

	t.Logf("Baseline: %v, Delayed: %v", baseline, delayed)
}

// TestMockServer_NetworkEnableFails verifies that Network.enable returns
// -32601 (simulating CloakBrowser which doesn't support Network domain).
func TestMockServer_NetworkEnableFails(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Close()

	b := rod.New().ControlURL(s.WSURL())
	if err := b.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() { _ = b.Close() }()

	err := (proto.NetworkEnable{}).Call(b)
	if err == nil {
		t.Fatal("expected Network.enable to fail (CloakBrowser simulation), got nil")
	}

	t.Logf("Network.enable failed as expected: %v", err)
}

// TestMockServer_CloseConnection simulates a Chrome crash — the WebSocket
// is forcibly closed mid-session.
func TestMockServer_CloseConnection(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Close()

	b := rod.New().ControlURL(s.WSURL())
	if err := b.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer func() { _ = b.Close() }()

	// First call works.
	_, err := (&proto.BrowserGetVersion{}).Call(b)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Simulate Chrome crash — close the WebSocket connection.
	s.CloseConnection()

	// Next call should fail (connection is gone).
	_, err = (&proto.BrowserGetVersion{}).Call(b)
	if err == nil {
		t.Fatal("expected error after CloseConnection, got nil")
	}

	t.Logf("Got expected error after crash: %v", err)
}

// TestMockServer_JSONVersion verifies the /json/version endpoint returns
// the correct webSocketDebuggerUrl.
func TestMockServer_JSONVersion(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Close()

	// Fetch /json/version via HTTP.
	resp, err := http.Get("http://127.0.0.1:" + port(s) + "/json/version")
	if err != nil {
		t.Fatalf("HTTP GET /json/version failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	wsURL, ok := v["webSocketDebuggerUrl"].(string)
	if !ok || wsURL == "" {
		t.Fatal("webSocketDebuggerUrl missing or empty")
	}

	if wsURL != s.WSURL() {
		t.Fatalf("webSocketDebuggerUrl mismatch: got %s, want %s", wsURL, s.WSURL())
	}

	t.Logf("Version endpoint OK: wsURL=%s", wsURL)
}

// --- helpers ---

func port(s *Server) string {
	// Extract port from wsURL (ws://127.0.0.1:PORT)
	u := s.WSURL()
	// strip "ws://127.0.0.1:"
	return u[len("ws://127.0.0.1:"):]
}
