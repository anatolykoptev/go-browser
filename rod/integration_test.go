package rod_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	rodbackend "github.com/anatolykoptev/go-browser/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// chromiumBin returns the Chromium binary path for integration tests.
func chromiumBin() string {
	if bin := os.Getenv("BROWSER_BIN"); bin != "" {
		return bin
	}
	for _, p := range []string{"/usr/bin/chromium-browser", "/usr/bin/chromium", "/usr/bin/google-chrome"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// skipIfNoChromium skips integration tests when Chromium is not available.
func skipIfNoChromium(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "" {
		return
	}
	if chromiumBin() != "" {
		return
	}
	if path, _ := launcher.LookPath(); path != "" {
		return
	}
	t.Skip("no Chromium found and INTEGRATION not set; skipping")
}

// newBrowser creates a Rod browser with common test options.
func newBrowser(t *testing.T, extra ...rodbackend.Option) *rodbackend.Browser {
	t.Helper()
	opts := []rodbackend.Option{rodbackend.WithHeadless(true)}
	if bin := chromiumBin(); bin != "" {
		opts = append(opts, rodbackend.WithBin(bin))
	}
	opts = append(opts, extra...)
	b, err := rodbackend.New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// testServer returns an httptest.Server serving pages for integration tests.
func testServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/simple", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Test Page</title></head><body><h1>Hello</h1></body></html>`)
	})

	mux.HandleFunc("/with-image", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Image Page</title></head>`+
			`<body><img id="img" src="/pixel.png" `+
			`onload="document.title='loaded'" onerror="document.title='blocked'">`+
			`<h1>Image Test</h1></body></html>`)
	})

	mux.HandleFunc("/pixel.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		// 1x1 red pixel PNG.
		w.Write([]byte{ //nolint:errcheck
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
			0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
			0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
			0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
			0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, 0x00, 0x00, 0x00,
			0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
		})
	})

	return httptest.NewServer(mux)
}

func TestIntegration_BasicRender(t *testing.T) {
	skipIfNoChromium(t)
	srv := testServer()
	defer srv.Close()

	b := newBrowser(t,
		func(o *rodbackend.Options) { o.Concurrency = 2 },
		func(o *rodbackend.Options) { o.RenderTimeout = 10 * time.Second },
	)

	if !b.Available() {
		t.Fatal("browser should be available after New()")
	}

	page, err := b.Render(context.Background(), srv.URL+"/simple")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if page.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", page.Title, "Test Page")
	}
	if !strings.Contains(page.HTML, "<h1>Hello</h1>") {
		t.Errorf("HTML missing <h1>Hello</h1>, got %d bytes", len(page.HTML))
	}
	t.Logf("rendered %d bytes, title=%q", len(page.HTML), page.Title)
}

func TestIntegration_ConcurrentRenders(t *testing.T) {
	skipIfNoChromium(t)
	srv := testServer()
	defer srv.Close()

	b := newBrowser(t,
		func(o *rodbackend.Options) { o.Concurrency = 2 },
		func(o *rodbackend.Options) { o.RenderTimeout = 15 * time.Second },
	)

	const n = 5
	var (
		wg      sync.WaitGroup
		success atomic.Int32
		errs    = make([]error, n)
	)

	wg.Add(n)
	for i := range n {
		go func(idx int) {
			defer wg.Done()
			page, err := b.Render(context.Background(), srv.URL+"/simple")
			if err != nil {
				errs[idx] = err
				return
			}
			if page.Title == "Test Page" {
				success.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if s := success.Load(); s != n {
		for i, e := range errs {
			if e != nil {
				t.Errorf("render[%d]: %v", i, e)
			}
		}
		t.Fatalf("success = %d/%d", s, n)
	}
	t.Logf("all %d concurrent renders succeeded", n)
}

func TestIntegration_ResourceBlocking(t *testing.T) {
	skipIfNoChromium(t)
	srv := testServer()
	defer srv.Close()

	// Control: render WITHOUT blocking — image loads, title becomes "loaded".
	bNoBlock := newBrowser(t,
		func(o *rodbackend.Options) { o.Concurrency = 1 },
		func(o *rodbackend.Options) { o.RenderTimeout = 10 * time.Second },
		func(o *rodbackend.Options) { o.HydrationWait = 2 * time.Second },
	)

	pageNoBlock, err := bNoBlock.Render(context.Background(), srv.URL+"/with-image")
	if err != nil {
		t.Fatalf("Render(no-block): %v", err)
	}
	if pageNoBlock.Title != "loaded" {
		t.Errorf("control: title = %q, want %q (image should load)", pageNoBlock.Title, "loaded")
	}
	_ = bNoBlock.Close()

	// Experiment: render WITH blocking — image blocked, title becomes "blocked".
	bBlock := newBrowser(t,
		rodbackend.WithBlockResources(rodbackend.ResourceImage),
		func(o *rodbackend.Options) { o.Concurrency = 1 },
		func(o *rodbackend.Options) { o.RenderTimeout = 10 * time.Second },
		func(o *rodbackend.Options) { o.HydrationWait = 2 * time.Second },
	)

	pageBlock, err := bBlock.Render(context.Background(), srv.URL+"/with-image")
	if err != nil {
		t.Fatalf("Render(block): %v", err)
	}
	if pageBlock.Title != "blocked" {
		t.Errorf("blocked: title = %q, want %q (image should be blocked)", pageBlock.Title, "blocked")
	}

	t.Logf("control title=%q, blocked title=%q", pageNoBlock.Title, pageBlock.Title)
}

func TestIntegration_CrashRecovery(t *testing.T) {
	skipIfNoChromium(t)
	srv := testServer()
	defer srv.Close()

	b := newBrowser(t,
		func(o *rodbackend.Options) { o.Concurrency = 1 },
		func(o *rodbackend.Options) { o.RenderTimeout = 15 * time.Second },
	)

	// Verify browser works before crash.
	page, err := b.Render(context.Background(), srv.URL+"/simple")
	if err != nil {
		t.Fatalf("pre-crash Render: %v", err)
	}
	if page.Title != "Test Page" {
		t.Fatalf("pre-crash Title = %q, want %q", page.Title, "Test Page")
	}

	// Kill the Chromium process to simulate a crash.
	pid := b.ChromiumPID()
	if pid == 0 {
		t.Fatal("ChromiumPID() = 0, cannot simulate crash")
	}
	t.Logf("killing Chromium PID %d", pid)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill(%d): %v", pid, err)
	}

	// Wait for process to die.
	time.Sleep(500 * time.Millisecond)

	// Render again — should auto-recover.
	page, err = b.Render(context.Background(), srv.URL+"/simple")
	if err != nil {
		t.Fatalf("post-crash Render failed (no recovery): %v", err)
	}
	if page.Title != "Test Page" {
		t.Errorf("post-crash Title = %q, want %q", page.Title, "Test Page")
	}

	// Verify new PID is different.
	newPID := b.ChromiumPID()
	if newPID == pid {
		t.Errorf("PID unchanged after crash: %d", newPID)
	}
	t.Logf("crash recovery succeeded: old PID=%d, new PID=%d", pid, newPID)
}
