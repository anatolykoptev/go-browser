package browser

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// localChromiumBin returns the Chromium binary path for integration tests.
func localChromiumBin() string {
	if bin := os.Getenv("BROWSER_BIN"); bin != "" {
		return bin
	}
	for _, p := range []string{
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/usr/bin/google-chrome",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// chromiumAvailable returns true if a Chromium binary can be found.
func chromiumAvailable() bool {
	if localChromiumBin() != "" {
		return true
	}
	path, _ := launcher.LookPath()
	return path != ""
}

// sharedBrowser holds a single Chromium instance shared across all stealth
// integration tests in this package. Tests that need a browser call
// acquireSharedBrowser(); if Chromium is unavailable the test is skipped.
var (
	sharedBrowserOnce     sync.Once
	sharedBrowserInstance *rod.Browser
	sharedBrowserLauncher *launcher.Launcher
	sharedBrowserErr      error
)

func acquireSharedBrowser(t *testing.T) *rod.Browser {
	t.Helper()

	wsURL := os.Getenv("CLOAKBROWSER_WS_URL")
	if wsURL == "" && os.Getenv("INTEGRATION") == "" && !chromiumAvailable() {
		t.Skip("no Chromium found; set CLOAKBROWSER_WS_URL or INTEGRATION")
	}

	sharedBrowserOnce.Do(func() {
		// Prefer remote cloakbrowser if URL is provided.
		if wsURL != "" {
			debuggerURL, err := launcher.ResolveURL(wsURL)
			if err != nil {
				sharedBrowserErr = err
				return
			}
			b := rod.New().ControlURL(debuggerURL)
			if err := b.Connect(); err != nil {
				sharedBrowserErr = err
				return
			}
			sharedBrowserInstance = b
			return
		}

		// Fallback: launch local Chromium.
		l := launcher.New().Headless(true).
			Set("no-sandbox").
			Set("disable-dev-shm-usage")
		if bin := localChromiumBin(); bin != "" {
			l = l.Bin(bin)
		}

		controlURL, err := l.Launch()
		if err != nil {
			sharedBrowserErr = err
			return
		}

		b := rod.New().ControlURL(controlURL)
		if err := b.Connect(); err != nil {
			l.Cleanup()
			sharedBrowserErr = err
			return
		}

		sharedBrowserInstance = b
		sharedBrowserLauncher = l
	})

	if sharedBrowserErr != nil {
		t.Skipf("Chromium unavailable: %v", sharedBrowserErr)
	}
	if sharedBrowserInstance == nil {
		t.Skip("Chromium launch failed; skipping stealth integration tests")
	}
	return sharedBrowserInstance
}

// TestStealthIntegration_Timezone verifies that CDP setTimezoneOverride causes
// Intl.DateTimeFormat to report the profile timezone.
func TestStealthIntegration_Timezone(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	m := &ChromeManager{browser: b}
	ctx, err := m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	page, err := m.NewStealthPage(ctx, profile)
	if err != nil {
		t.Fatalf("NewStealthPage: %v", err)
	}
	defer func() { _ = page.Close() }()

	// Navigate to a data URL that sets the title to the detected timezone.
	if err := page.Navigate(`data:text/html,<script>document.title = Intl.DateTimeFormat().resolvedOptions().timeZone;</script>`); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_ = page.WaitLoad()

	info, err := page.Info()
	if err != nil {
		t.Fatalf("page.Info: %v", err)
	}

	if info.Title != profile.Timezone {
		t.Errorf("timezone = %q, want %q", info.Title, profile.Timezone)
	}
	t.Logf("timezone verified: %q", info.Title)
}

// TestStealthIntegration_Locale verifies that CDP setLocaleOverride causes
// navigator.language to match the profile's primary language.
func TestStealthIntegration_Locale(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	m := &ChromeManager{browser: b}
	ctx, err := m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	page, err := m.NewStealthPage(ctx, profile)
	if err != nil {
		t.Fatalf("NewStealthPage: %v", err)
	}
	defer func() { _ = page.Close() }()

	if err := page.Navigate(`data:text/html,<script>document.title = navigator.language + "|" + navigator.languages.join(",");</script>`); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_ = page.WaitLoad()

	info, err := page.Info()
	if err != nil {
		t.Fatalf("page.Info: %v", err)
	}

	parts := strings.SplitN(info.Title, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected title format: %q", info.Title)
	}

	wantLang := profile.Langs[0]
	if parts[0] != wantLang {
		t.Errorf("navigator.language = %q, want %q", parts[0], wantLang)
	}

	wantLangs := strings.Join(profile.Langs, ",")
	if parts[1] != wantLangs {
		t.Errorf("navigator.languages = %q, want %q", parts[1], wantLangs)
	}
	t.Logf("locale verified: language=%q languages=%q", parts[0], parts[1])
}

// TestStealthIntegration_IframeStealth verifies that child iframes do not expose
// navigator.webdriver = true, proving stealth injections propagate via setAutoAttach.
func TestStealthIntegration_IframeStealth(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	m := &ChromeManager{browser: b}
	ctx, err := m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	page, err := m.NewStealthPage(ctx, profile)
	if err != nil {
		t.Fatalf("NewStealthPage: %v", err)
	}
	defer func() { _ = page.Close() }()

	// Page that creates an iframe and reads navigator.webdriver from inside it.
	// The iframe writes its result as the document title of the parent.
	html := `data:text/html,<script>
var f = document.createElement('iframe');
f.src = 'about:blank';
document.body.appendChild(f);
setTimeout(function() {
	try {
		var wd = f.contentWindow.navigator.webdriver;
		document.title = 'webdriver=' + String(wd);
	} catch(e) {
		document.title = 'error:' + e.message;
	}
}, 300);
</script>`

	if err := page.Navigate(html); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	// Wait for the setTimeout to run.
	_ = page.WaitIdle(time.Second)

	info, err := page.Info()
	if err != nil {
		t.Fatalf("page.Info: %v", err)
	}

	// navigator.webdriver should be false (or undefined) inside the iframe.
	// If stealth is working, it should NOT be "webdriver=true".
	if info.Title == "webdriver=true" {
		t.Errorf("iframe still has webdriver=true — stealth not propagated to iframe")
	}
	t.Logf("iframe stealth verified: title=%q", info.Title)
}

// TestStealthIntegration_AcceptLanguageHeader verifies that the Accept-Language
// HTTP header matches the profile's primary language tag.
func TestStealthIntegration_AcceptLanguageHeader(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	var receivedLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedLang = r.Header.Get("Accept-Language")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	}))
	defer srv.Close()

	m := &ChromeManager{browser: b}
	ctx, err := m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	page, err := m.NewStealthPage(ctx, profile)
	if err != nil {
		t.Fatalf("NewStealthPage: %v", err)
	}
	defer func() { _ = page.Close() }()

	if err := page.Navigate(srv.URL); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_ = page.WaitLoad()

	if receivedLang == "" {
		t.Fatal("Accept-Language header was not received by test server")
	}

	// The header must start with the profile's primary language.
	primaryLang := profile.Langs[0]
	if !strings.HasPrefix(receivedLang, primaryLang) {
		t.Errorf("Accept-Language = %q, want prefix %q", receivedLang, primaryLang)
	}
	t.Logf("Accept-Language verified: %q", receivedLang)
}
