package browser

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/anatolykoptev/go-browser/selftest"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// handleSelftest runs antibot probe targets against the live CloakBrowser instance
// and returns a structured JSON trust report.
//
// Query parameters:
//
//	target     — comma-separated list of probe keys, or "all" / omitted for all targets
//	profile    — stealth profile name (default: mac_chrome145)
//	screenshot — set to "1" to save full-page PNGs under /tmp/selftest/
func (s *Server) handleSelftest(w http.ResponseWriter, r *http.Request) {
	if s.chrome == nil || !s.chrome.Connected() {
		writeError(w, http.StatusServiceUnavailable, "chrome not connected")
		return
	}

	targetParam := r.URL.Query().Get("target")
	profileParam := r.URL.Query().Get("profile")
	screenshotParam := r.URL.Query().Get("screenshot")

	var targets []string
	if targetParam != "" && targetParam != "all" {
		for _, t := range strings.Split(targetParam, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				targets = append(targets, t)
			}
		}
	}

	screenshot := screenshotParam == "1"

	factory := makePageFactory(s.chrome, profileParam)

	report, err := selftest.Run(r.Context(), factory, targets, profileParam, screenshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("selftest: %s", err))
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// makePageFactory returns a selftest.PageFactory that creates an isolated stealth
// page via chrome for each probe target. The returned cleanup function disposes
// the browser context and closes the page.
func makePageFactory(chrome *ChromeManager, profileName string) selftest.PageFactory {
	return func(_ string) (*rod.Page, func(), error) {
		profile, err := LoadProfile(profileName)
		if err != nil {
			return nil, nil, fmt.Errorf("load profile %q: %w", profileName, err)
		}

		b, contextID, authCleanup, err := chrome.NewContext("")
		if err != nil {
			return nil, nil, fmt.Errorf("create context: %w", err)
		}

		page, err := chrome.NewStealthPage(b, profile)
		if err != nil {
			_ = proto.TargetDisposeBrowserContext{BrowserContextID: contextID}.Call(b)
			if authCleanup != nil {
				authCleanup()
			}
			return nil, nil, fmt.Errorf("create stealth page: %w", err)
		}

		cleanup := func() {
			_ = page.Close()
			_ = proto.TargetDisposeBrowserContext{BrowserContextID: contextID}.Call(b)
			if authCleanup != nil {
				authCleanup()
			}
		}
		return page, cleanup, nil
	}
}
