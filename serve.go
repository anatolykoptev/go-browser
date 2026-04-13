package browser

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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
	mux.HandleFunc("POST /chrome/interact", s.handleInteract)
	mux.HandleFunc("POST /solve", s.handleSolve)
	mux.HandleFunc("POST /render", s.handleRender)
	mux.HandleFunc("DELETE /session/{id}", s.handleDestroySession)
	mux.HandleFunc("GET /selftest", s.handleSelftest)
}

// handleHealth returns 200 OK with service status.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
