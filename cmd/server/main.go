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
	srv := browser.NewServer(cfg, logger)

	logger.Info("starting go-browser server", "port", cfg.Port, "cloakbrowser_ws", cfg.CloakBrowserWSURL)

	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}
