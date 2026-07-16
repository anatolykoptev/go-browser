package browser

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

const (
	defaultPort              = "8906"
	defaultCloakBrowserWSURL = "ws://127.0.0.1:9222"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Port              string
	CloakBrowserWSURL string
}

// ServerConfigFromEnv reads ServerConfig from environment variables.
// PORT defaults to 8906, CLOAKBROWSER_WS_URL defaults to ws://127.0.0.1:9222.
func ServerConfigFromEnv() ServerConfig {
	cfg := ServerConfig{
		Port:              defaultPort,
		CloakBrowserWSURL: defaultCloakBrowserWSURL,
	}
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("CLOAKBROWSER_WS_URL"); v != "" {
		cfg.CloakBrowserWSURL = v
	}
	return cfg
}

// Server is the go-browser HTTP API server.
type Server struct {
	cfg    ServerConfig
	mux    *http.ServeMux
	chrome *ChromeManager
	logger *slog.Logger
}

// NewServer constructs a Server, connects to CloakBrowser, and registers all routes.
func NewServer(cfg ServerConfig, logger *slog.Logger) (*Server, error) {
	chrome, err := NewChromeManager(cfg.CloakBrowserWSURL)
	if err != nil {
		logger.Warn("chrome not available — Chrome endpoints will return 503", "err", err)
	}

	mux := http.NewServeMux()
	s := &Server{
		cfg:    cfg,
		mux:    mux,
		chrome: chrome,
		logger: logger,
	}
	s.registerRoutes(mux)
	return s, nil
}

// ListenAndServe starts the HTTP server and blocks until it returns an error.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%s", s.cfg.Port)
	s.logger.Info("listening", "addr", addr)
	return http.ListenAndServe(addr, s.mux) //nolint:gosec // non-TLS intentional for internal sidecar
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleMetrics) // #55: Prometheus metrics
	mux.HandleFunc("POST /chrome/interact", s.handleInteract)
	mux.HandleFunc("POST /solve", s.handleSolve)
	mux.HandleFunc("POST /render", s.handleRender)
	mux.HandleFunc("DELETE /session/{id}", s.handleDestroySession)
	mux.HandleFunc("GET /selftest", s.handleSelftest)
}

// handleHealth returns 200 OK with structured health status.
// #49: Uses HealthCheck() for active CDP probe with latency measurement.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	if s.chrome == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":    "degraded",
			"connected": false,
			"error":     "chrome not connected",
		})
		return
	}
	status := s.chrome.HealthCheck()
	code := http.StatusOK
	if !status.Connected {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, status)
}

// handleMetrics exposes Prometheus-format metrics.
// #55: Text exposition format — no prometheus/client_golang dependency needed.
// Metrics exposed:
//   - go_browser_connected (gauge: 1=connected, 0=disconnected)
//   - go_browser_cdp_latency_ms (gauge: CDP round-trip latency)
//   - go_browser_context_count (gauge: active browser contexts)
//   - go_browser_page_count (gauge: active managed pages)
//   - go_browser_generation (gauge: context pool generation counter)
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var sb strings.Builder
	if s.chrome == nil {
		sb.WriteString("# HELP go_browser_connected Chrome connection state (1=connected, 0=disconnected)\n")
		sb.WriteString("# TYPE go_browser_connected gauge\n")
		sb.WriteString("go_browser_connected 0\n")
		_, _ = w.Write([]byte(sb.String()))
		return
	}

	status := s.chrome.HealthCheck()

	connected := 0
	if status.Connected {
		connected = 1
	}

	sb.WriteString("# HELP go_browser_connected Chrome connection state (1=connected, 0=disconnected)\n")
	sb.WriteString("# TYPE go_browser_connected gauge\n")
	fmt.Fprintf(&sb, "go_browser_connected %d\n", connected)

	sb.WriteString("# HELP go_browser_cdp_latency_ms CDP round-trip latency in milliseconds\n")
	sb.WriteString("# TYPE go_browser_cdp_latency_ms gauge\n")
	fmt.Fprintf(&sb, "go_browser_cdp_latency_ms %d\n", status.LatencyMs)

	if status.ContextPool != nil {
		sb.WriteString("# HELP go_browser_context_count Active browser contexts\n")
		sb.WriteString("# TYPE go_browser_context_count gauge\n")
		fmt.Fprintf(&sb, "go_browser_context_count %d\n", status.ContextPool.Contexts)

		sb.WriteString("# HELP go_browser_page_count Active managed pages\n")
		sb.WriteString("# TYPE go_browser_page_count gauge\n")
		fmt.Fprintf(&sb, "go_browser_page_count %d\n", status.ContextPool.Pages)

		sb.WriteString("# HELP go_browser_generation Context pool generation counter (increments on reconnect)\n")
		sb.WriteString("# TYPE go_browser_generation gauge\n")
		fmt.Fprintf(&sb, "go_browser_generation %d\n", status.ContextPool.Generation)
	}

	_, _ = w.Write([]byte(sb.String()))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
