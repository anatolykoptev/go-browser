// Command server runs the go-browser HTTP API server.
package main

import (
	"log/slog"
	"os"

	browser "github.com/anatolykoptev/go-browser"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg := browser.ServerConfigFromEnv()
	logger.Info("starting go-browser server", "port", cfg.Port, "cloakbrowser_ws", cfg.CloakBrowserWSURL)

	srv, err := browser.NewServer(cfg, logger)
	if err != nil {
		logger.Error("server init failed", "error", err)
		os.Exit(1)
	}

	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}
