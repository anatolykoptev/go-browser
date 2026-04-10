package rod

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// isConnectionError returns true if the error indicates a dead Chromium process.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "websocket close") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "EOF")
}

// restart closes the dead browser and launches a fresh Chromium with the same options.
func (b *Browser) restart() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.rod != nil {
		b.rod.Close() //nolint:errcheck // dying browser, best-effort
	}
	if b.launcher != nil {
		b.launcher.Kill() // clean up zombie Chromium
	}

	l := launcher.New().Headless(b.opts.Headless).
		Set("disable-blink-features", "AutomationControlled")
	if b.opts.Bin != "" {
		l = l.Bin(b.opts.Bin)
	}
	if b.opts.ProxyPool != nil {
		if proxy := b.opts.ProxyPool.Next(); proxy != "" {
			l = l.Proxy(proxy)
		}
	}

	controlURL, err := l.Launch()
	if err != nil {
		b.rod = nil
		b.launcher = nil
		return fmt.Errorf("rod: restart launch: %w", err)
	}

	newBrowser := rod.New().ControlURL(controlURL)
	if err := newBrowser.Connect(); err != nil {
		b.rod = nil
		b.launcher = nil
		return fmt.Errorf("rod: restart connect: %w", err)
	}

	b.rod = newBrowser
	b.launcher = l
	slog.Info("rod: browser restarted")
	return nil
}
