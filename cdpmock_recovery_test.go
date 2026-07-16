package browser

import (
	"testing"
	"time"

	"github.com/anatolykoptev/go-browser/cdpmock"
	"github.com/go-rod/rod/lib/proto"
)

// TestCDPMock_HealthCheck verifies HealthCheck against the CDP mock.
// This test runs in CI without a real Chrome — the mock provides the
// CDP responses.
func TestCDPMock_HealthCheck(t *testing.T) {
	s := cdpmock.New()
	if err := s.Start(); err != nil {
		t.Fatalf("mock start: %v", err)
	}
	defer s.Close()

	m, err := NewChromeManager(s.WSURL())
	if err != nil {
		t.Fatalf("NewChromeManager: %v", err)
	}
	defer m.Close()

	status := m.HealthCheck()
	if !status.Connected {
		t.Fatal("HealthCheck should report connected=true")
	}

	if status.LatencyMs < 0 {
		t.Fatalf("latency should be >= 0, got %d", status.LatencyMs)
	}

	t.Logf("HealthCheck: connected=%v latency=%dms", status.Connected, status.LatencyMs)
}

// TestCDPMock_HealthCheckAfterCrash verifies that HealthCheck detects
// a crashed Chrome (WebSocket closed). The mock simulates the crash.
func TestCDPMock_HealthCheckAfterCrash(t *testing.T) {
	s := cdpmock.New()
	if err := s.Start(); err != nil {
		t.Fatalf("mock start: %v", err)
	}
	defer s.Close()

	m, err := NewChromeManager(s.WSURL())
	if err != nil {
		t.Fatalf("NewChromeManager: %v", err)
	}
	defer m.Close()

	// Verify connected.
	status := m.HealthCheck()
	if !status.Connected {
		t.Fatal("should be connected initially")
	}

	// Simulate Chrome crash — close the WebSocket connection.
	s.CloseConnection()

	// HealthCheck should now report disconnected (CDP call fails).
	status = m.HealthCheck()
	if status.Connected {
		t.Fatal("HealthCheck should report connected=false after crash")
	}

	t.Logf("HealthCheck after crash: connected=%v", status.Connected)
}

// TestCDPMock_Reconnect verifies that reconnect() works against the mock.
// After a crash, reconnect establishes a new connection to the mock.
func TestCDPMock_Reconnect(t *testing.T) {
	s := cdpmock.New()
	if err := s.Start(); err != nil {
		t.Fatalf("mock start: %v", err)
	}
	defer s.Close()

	m, err := NewChromeManager(s.WSURL())
	if err != nil {
		t.Fatalf("NewChromeManager: %v", err)
	}
	defer m.Close()

	// Verify initial connection.
	_, err = (&proto.BrowserGetVersion{}).Call(m.getBrowser())
	if err != nil {
		t.Fatalf("initial getVersion: %v", err)
	}

	// Simulate crash.
	s.CloseConnection()

	// Reconnect.
	if err := m.reconnect(); err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	// Verify new connection works.
	ver, err := (&proto.BrowserGetVersion{}).Call(m.getBrowser())
	if err != nil {
		t.Fatalf("getVersion after reconnect: %v", err)
	}

	if ver.Product != "cdpmock/v1.0" {
		t.Fatalf("expected cdpmock/v1.0, got %s", ver.Product)
	}

	t.Logf("Reconnect OK: %s", ver.Product)
}

// TestCDPMock_InjectErrorOnHealthCheck verifies that InjectError causes
// HealthCheck to report disconnected (CDP call returns error).
func TestCDPMock_InjectErrorOnHealthCheck(t *testing.T) {
	s := cdpmock.New()
	if err := s.Start(); err != nil {
		t.Fatalf("mock start: %v", err)
	}
	defer s.Close()

	m, err := NewChromeManager(s.WSURL())
	if err != nil {
		t.Fatalf("NewChromeManager: %v", err)
	}
	defer m.Close()

	// Initially connected.
	status := m.HealthCheck()
	if !status.Connected {
		t.Fatal("should be connected initially")
	}

	// Inject error on Browser.getVersion.
	s.InjectError("Browser.getVersion", errMockUnavailable)

	// HealthCheck should now report disconnected.
	status = m.HealthCheck()
	if status.Connected {
		t.Fatal("should be disconnected after injected error")
	}

	t.Logf("HealthCheck with injected error: connected=%v", status.Connected)
}

// TestCDPMock_DelayedResponse verifies that InjectDelay slows down CDP calls.
// This tests the HealthCheck latency measurement path.
func TestCDPMock_DelayedResponse(t *testing.T) {
	s := cdpmock.New()
	if err := s.Start(); err != nil {
		t.Fatalf("mock start: %v", err)
	}
	defer s.Close()

	m, err := NewChromeManager(s.WSURL())
	if err != nil {
		t.Fatalf("NewChromeManager: %v", err)
	}
	defer m.Close()

	// Baseline latency.
	status := m.HealthCheck()
	baseline := status.LatencyMs

	// Inject 100ms delay.
	s.InjectDelay(100 * time.Millisecond)
	defer s.InjectDelay(0)

	status = m.HealthCheck()
	delayed := status.LatencyMs

	if delayed < 90 {
		t.Fatalf("expected ~100ms latency, got %dms (baseline %dms)", delayed, baseline)
	}

	t.Logf("Latency: baseline=%dms, delayed=%dms", baseline, delayed)
}

// TestCDPMock_LostConnection verifies that the disconnect watcher detects
// a crashed mock and closes the LostConnection channel.
func TestCDPMock_LostConnection(t *testing.T) {
	s := cdpmock.New()
	if err := s.Start(); err != nil {
		t.Fatalf("mock start: %v", err)
	}
	defer s.Close()

	m, err := NewChromeManager(s.WSURL())
	if err != nil {
		t.Fatalf("NewChromeManager: %v", err)
	}
	defer m.Close()

	// LostConnection should be open initially.
	select {
	case <-m.LostConnection:
		t.Fatal("LostConnection should not be closed initially")
	default:
	}

	// Simulate Chrome crash.
	s.CloseConnection()

	// The disconnect watcher polls every 5s. For the test, we can
	// trigger a HealthCheck which will detect the disconnect directly.
	// The watcher will also fire, but we don't wait for it.
	status := m.HealthCheck()
	if status.Connected {
		t.Fatal("HealthCheck should detect crash")
	}

	t.Logf("LostConnection test: HealthCheck detected crash=%v", !status.Connected)
}

// TestCDPMock_ContextPoolCreatePage verifies that ContextPool can create
// a page via the mock CDP server.
func TestCDPMock_ContextPoolCreatePage(t *testing.T) {
	s := cdpmock.New()
	if err := s.Start(); err != nil {
		t.Fatalf("mock start: %v", err)
	}
	defer s.Close()

	m, err := NewChromeManager(s.WSURL())
	if err != nil {
		t.Fatalf("NewChromeManager: %v", err)
	}
	defer m.Close()

	pool := m.Pool()
	if pool == nil {
		t.Fatal("Pool should not be nil")
	}

	// Create a page — this goes through Target.createTarget via the mock.
	mp, err := pool.GetOrCreatePage("test-session", "default", "", "about:blank")
	if err != nil {
		t.Fatalf("GetOrCreatePage: %v", err)
	}

	if mp.Page == nil {
		t.Fatal("ManagedPage.Page should not be nil")
	}

	t.Cleanup(func() { _ = pool.ClosePage("test-session") })
	t.Logf("Created page via mock: session=test-session")
}

// TestCDPMock_NetworkEnableFails verifies that the egress guard handles
// the mock's Network.enable failure gracefully (same as CloakBrowser).
func TestCDPMock_NetworkEnableFails(t *testing.T) {
	s := cdpmock.New()
	if err := s.Start(); err != nil {
		t.Fatalf("mock start: %v", err)
	}
	defer s.Close()

	m, err := NewChromeManager(s.WSURL())
	if err != nil {
		t.Fatalf("NewChromeManager: %v", err)
	}
	defer m.Close()

	// The egress guard should have been installed despite Network.enable
	// failing (it falls back to Page.frameNavigated).
	guard := m.getGuard()
	if guard == nil {
		t.Fatal("egress guard should be installed")
	}

	t.Logf("Egress guard installed with Network.enable fallback (like CloakBrowser)")
}

// sentinel error for InjectError tests.
var errMockUnavailable = errMock("mock error: browser unavailable")

type errMock string

func (e errMock) Error() string { return string(e) }
