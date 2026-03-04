# go-browser Phase 0 + Phase 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create go-browser library with Browser interface, Rod backend, and migrate go-search + go-wp off chromedp + browserless container.

**Architecture:** Shared `Browser` interface with pluggable backends. Phase 0 defines the interface + remote CDP backend (drop-in for current chromedp code). Phase 1 adds Rod backend (in-process Chromium, eliminates browserless container). Consumers import go-browser and pass `Browser` via dependency injection.

**Tech Stack:** Go 1.26, `github.com/go-rod/rod`, `github.com/chromedp/chromedp` (remote backend only), `github.com/anatolykoptev/go-stealth` (proxy pool)

**Source:** `/home/krolik/src/go-browser/`
**Module:** `github.com/anatolykoptev/go-browser`

---

### Task 1: Bootstrap Module + Interface

**Files:**
- Create: `go.mod`
- Create: `browser.go`
- Create: `options.go`
- Create: `errors.go`

**Step 1: Initialize Go module**

```bash
cd /home/krolik/src/go-browser
go mod init github.com/anatolykoptev/go-browser
```

**Step 2: Write browser.go — core interface + Page type**

```go
// Package browser provides a unified interface for headless browser automation.
package browser

import "context"

// Browser renders web pages via a headless browser backend.
type Browser interface {
	// Render navigates to url, waits for JS hydration, returns rendered page.
	Render(ctx context.Context, url string) (*Page, error)

	// Available reports whether the backend is connected and usable.
	Available() bool

	// Close shuts down the browser and releases resources.
	Close() error
}

// Page holds the result of a rendered page.
type Page struct {
	URL    string // Final URL after redirects.
	HTML   string // Rendered outerHTML.
	Title  string // Page <title>.
	Status int    // HTTP status code (0 if unknown).
}
```

**Step 3: Write options.go — common options**

```go
package browser

import "time"

// Options holds common configuration for all backends.
type Options struct {
	Concurrency    int
	RenderTimeout  time.Duration
	HydrationWait  time.Duration
	UserAgent      string
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{
		Concurrency:   3,
		RenderTimeout: 20 * time.Second,
		HydrationWait: 2 * time.Second,
	}
}

// Option configures browser behavior.
type Option func(*Options)

// WithConcurrency sets maximum concurrent renders.
func WithConcurrency(n int) Option {
	return func(o *Options) { o.Concurrency = n }
}

// WithRenderTimeout sets per-page render deadline.
func WithRenderTimeout(d time.Duration) Option {
	return func(o *Options) { o.RenderTimeout = d }
}

// WithHydrationWait sets the delay after body ready before capturing HTML.
func WithHydrationWait(d time.Duration) Option {
	return func(o *Options) { o.HydrationWait = d }
}

// WithUserAgent overrides the browser User-Agent header.
func WithUserAgent(ua string) Option {
	return func(o *Options) { o.UserAgent = ua }
}

// Apply applies options over defaults.
func Apply(opts ...Option) Options {
	o := DefaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
```

**Step 4: Write errors.go — sentinel errors**

```go
package browser

import "errors"

var (
	// ErrUnavailable means the backend is not connected or binary not found.
	ErrUnavailable = errors.New("browser: backend unavailable")

	// ErrTimeout means the render exceeded the configured deadline.
	ErrTimeout = errors.New("browser: render timeout")

	// ErrNavigate means page navigation failed (DNS, TLS, HTTP error).
	ErrNavigate = errors.New("browser: navigation failed")
)
```

**Step 5: Commit**

```bash
cd /home/krolik/src/go-browser
git init && git add -A
git commit -m "feat: bootstrap go-browser module with Browser interface

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Page Pool (Semaphore)

**Files:**
- Create: `pool.go`
- Create: `pool_test.go`

**Step 1: Write the failing test**

```go
package browser_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-browser"
)

func TestPool_LimitsConcurrency(t *testing.T) {
	pool := browser.NewPool(2)
	defer pool.Close()

	var running atomic.Int32
	var maxSeen atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := pool.Acquire(context.Background())
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			defer release()

			cur := running.Add(1)
			for {
				old := maxSeen.Load()
				if cur <= old || maxSeen.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			running.Add(-1)
		}()
	}
	wg.Wait()

	if maxSeen.Load() > 2 {
		t.Errorf("max concurrent = %d, want <= 2", maxSeen.Load())
	}
}

func TestPool_RespectsContextCancel(t *testing.T) {
	pool := browser.NewPool(1)
	defer pool.Close()

	// Fill the pool.
	release, _ := pool.Acquire(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := pool.Acquire(ctx)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	release()
}
```

**Step 2: Run test to verify it fails**

```bash
cd /home/krolik/src/go-browser && go test -run TestPool -v
```

Expected: FAIL — `NewPool` not defined.

**Step 3: Write pool.go**

```go
package browser

import "context"

// Pool limits concurrent browser operations via a semaphore.
type Pool struct {
	sem  chan struct{}
	done chan struct{}
}

// NewPool creates a pool with the given concurrency limit.
func NewPool(size int) *Pool {
	if size < 1 {
		size = 1
	}
	return &Pool{
		sem:  make(chan struct{}, size),
		done: make(chan struct{}),
	}
}

// Acquire blocks until a slot is available or ctx is cancelled.
// Returns a release function that must be called when done.
func (p *Pool) Acquire(ctx context.Context) (release func(), err error) {
	select {
	case p.sem <- struct{}{}:
		return func() { <-p.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return nil, ErrUnavailable
	}
}

// Close signals all waiters to abort.
func (p *Pool) Close() {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
}
```

**Step 4: Run tests**

```bash
cd /home/krolik/src/go-browser && go test -run TestPool -v -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
cd /home/krolik/src/go-browser
git add pool.go pool_test.go
git commit -m "feat: add page pool with semaphore concurrency control

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Rod Backend

**Files:**
- Create: `rod/rod.go`
- Create: `rod/options.go`
- Create: `rod/rod_test.go`

**Step 1: Write rod/options.go**

```go
package rod

import (
	browser "github.com/anatolykoptev/go-browser"
	stealth "github.com/anatolykoptev/go-stealth"
)

// Options extends common browser options with Rod-specific settings.
type Options struct {
	browser.Options
	Bin       string                    // Custom Chromium binary path (empty = auto-download).
	ProxyPool stealth.ProxyPoolProvider // Rotating proxy pool from go-stealth.
	Headless  bool                      // Run in headless mode (default true).
}

// DefaultOptions returns Rod defaults.
func DefaultOptions() Options {
	return Options{
		Options:  browser.DefaultOptions(),
		Headless: true,
	}
}

// Option configures the Rod backend.
type Option func(*Options)

// WithBin sets a custom Chromium binary path.
func WithBin(path string) Option {
	return func(o *Options) { o.Bin = path }
}

// WithProxyPool sets the go-stealth proxy pool for rotation.
func WithProxyPool(pool stealth.ProxyPoolProvider) Option {
	return func(o *Options) { o.ProxyPool = pool }
}

// WithHeadless toggles headless mode (default true).
func WithHeadless(v bool) Option {
	return func(o *Options) { o.Headless = v }
}
```

**Step 2: Write rod/rod.go**

```go
// Package rod provides a Browser backend using go-rod/rod (in-process Chromium).
package rod

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	browser "github.com/anatolykoptev/go-browser"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// Browser implements browser.Browser using Rod.
type Browser struct {
	rod  *rod.Browser
	pool *browser.Pool
	opts Options
}

// New creates a Rod browser backend.
// It launches Chromium (or connects to an existing binary).
func New(opts ...Option) (*Browser, error) {
	o := DefaultOptions()
	for _, fn := range opts {
		fn(&o)
	}

	l := launcher.New().Headless(o.Headless)
	if o.Bin != "" {
		l = l.Bin(o.Bin)
	}
	if o.ProxyPool != nil {
		if proxy := o.ProxyPool.Next(); proxy != "" {
			l = l.Proxy(proxy)
		}
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("rod: launch: %w", err)
	}

	b := rod.New().ControlURL(controlURL)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("rod: connect: %w", err)
	}

	slog.Info("rod: browser launched", "headless", o.Headless)

	return &Browser{
		rod:  b,
		pool: browser.NewPool(o.Concurrency),
		opts: o,
	}, nil
}

// Render navigates to url, waits for JS, returns rendered HTML.
func (b *Browser) Render(ctx context.Context, url string) (*browser.Page, error) {
	if b.rod == nil {
		return nil, browser.ErrUnavailable
	}

	release, err := b.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	renderCtx, cancel := context.WithTimeout(ctx, b.opts.RenderTimeout)
	defer cancel()

	page, err := b.rod.Context(renderCtx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", browser.ErrNavigate, err)
	}
	defer page.Close()

	if err := page.Navigate(url); err != nil {
		return nil, fmt.Errorf("%w: %v", browser.ErrNavigate, err)
	}

	if err := page.WaitDOMStable(b.opts.HydrationWait, 0.1); err != nil {
		return nil, fmt.Errorf("%w: %v", browser.ErrTimeout, err)
	}

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("rod: html: %w", err)
	}

	info, _ := page.Info()
	title := ""
	if info != nil {
		title = info.Title
	}

	return &browser.Page{
		URL:   page.MustInfo().URL,
		HTML:  html,
		Title: title,
	}, nil
}

// Available reports whether the Rod browser is connected.
func (b *Browser) Available() bool {
	return b.rod != nil
}

// Close shuts down Rod and the Chromium process.
func (b *Browser) Close() error {
	b.pool.Close()
	if b.rod != nil {
		return b.rod.Close()
	}
	return nil
}
```

**Step 3: Add dependencies**

```bash
cd /home/krolik/src/go-browser
go get github.com/go-rod/rod@latest
go get github.com/anatolykoptev/go-stealth@latest
go mod tidy
```

**Step 4: Write rod/rod_test.go — integration test**

```go
package rod_test

import (
	"context"
	"strings"
	"testing"
	"time"

	browser "github.com/anatolykoptev/go-browser"
	rodbackend "github.com/anatolykoptev/go-browser/rod"
)

func TestRod_RenderPage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	b, err := rodbackend.New(
		func(o *rodbackend.Options) { o.Concurrency = 2 },
		func(o *rodbackend.Options) { o.RenderTimeout = 15 * time.Second },
	)
	if err != nil {
		t.Fatalf("new rod: %v", err)
	}
	defer b.Close()

	if !b.Available() {
		t.Fatal("browser not available")
	}

	page, err := b.Render(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(page.HTML, "Example Domain") {
		t.Errorf("expected 'Example Domain' in HTML, got %d bytes", len(page.HTML))
	}
	if page.Title != "Example Domain" {
		t.Errorf("title = %q, want 'Example Domain'", page.Title)
	}
}

func TestRod_Unavailable(t *testing.T) {
	b := &rodbackend.Browser{}
	if b.Available() {
		t.Error("zero-value browser should not be available")
	}
	_, err := b.Render(context.Background(), "https://example.com")
	if err != browser.ErrUnavailable {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}
```

Note: `TestRod_Unavailable` requires exporting the zero-value check. Alternative: skip if `Available()` returns false.

**Step 5: Run unit tests (skip integration)**

```bash
cd /home/krolik/src/go-browser && go test ./... -short -v -count=1
```

Expected: pool tests PASS, rod integration tests SKIP.

**Step 6: Run integration test (needs Chromium)**

```bash
cd /home/krolik/src/go-browser && go test ./rod/ -v -count=1 -timeout 30s
```

Expected: PASS (Rod auto-downloads Chromium on first run).

**Step 7: Commit**

```bash
cd /home/krolik/src/go-browser
git add rod/ go.mod go.sum
git commit -m "feat: add Rod backend (in-process Chromium)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: Remote CDP Backend

**Files:**
- Create: `remote/remote.go`
- Create: `remote/options.go`

**Step 1: Write remote/options.go**

```go
package remote

import browser "github.com/anatolykoptev/go-browser"

// Options for remote CDP backend.
type Options struct {
	browser.Options
	Endpoint string // WebSocket URL (e.g. ws://browserless:3000).
}

// Option configures the remote backend.
type Option func(*Options)

// WithEndpoint sets the CDP WebSocket endpoint.
func WithEndpoint(url string) Option {
	return func(o *Options) { o.Endpoint = url }
}
```

**Step 2: Write remote/remote.go**

```go
// Package remote provides a Browser backend connecting to an external CDP endpoint.
// Compatible with Browserless, Lightpanda, or any CDP-compatible server.
package remote

import (
	"context"
	"fmt"
	"log/slog"

	browser "github.com/anatolykoptev/go-browser"
	"github.com/chromedp/chromedp"
)

// Browser implements browser.Browser via a remote CDP WebSocket.
type Browser struct {
	ctx         context.Context
	allocCancel context.CancelFunc
	ctxCancel   context.CancelFunc
	pool        *browser.Pool
	opts        Options
	connected   bool
}

// New connects to a remote CDP endpoint.
func New(opts ...Option) (*Browser, error) {
	o := Options{Options: browser.DefaultOptions()}
	for _, fn := range opts {
		fn(&o)
	}

	if o.Endpoint == "" {
		return &Browser{}, nil
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(
		context.Background(), o.Endpoint, chromedp.NoModifyURL,
	)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	if err := chromedp.Run(ctx); err != nil {
		allocCancel()
		ctxCancel()
		return nil, fmt.Errorf("remote: connect %s: %w", o.Endpoint, err)
	}

	slog.Info("remote: connected", "endpoint", o.Endpoint)

	return &Browser{
		ctx:         ctx,
		allocCancel: allocCancel,
		ctxCancel:   ctxCancel,
		pool:        browser.NewPool(o.Concurrency),
		opts:        o,
		connected:   true,
	}, nil
}

// Render navigates to url, waits, returns rendered HTML.
func (b *Browser) Render(ctx context.Context, url string) (*browser.Page, error) {
	if !b.connected {
		return nil, browser.ErrUnavailable
	}

	release, err := b.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	tabCtx, tabCancel := chromedp.NewContext(b.ctx)
	defer tabCancel()

	renderCtx, renderCancel := context.WithTimeout(tabCtx, b.opts.RenderTimeout)
	defer renderCancel()

	var html string
	err = chromedp.Run(renderCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(b.opts.HydrationWait),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", browser.ErrNavigate, err)
	}

	var title string
	_ = chromedp.Run(renderCtx, chromedp.Title(&title))

	return &browser.Page{
		URL:   url,
		HTML:  html,
		Title: title,
	}, nil
}

// Available reports whether the remote browser is connected.
func (b *Browser) Available() bool { return b.connected }

// Close disconnects from the remote browser.
func (b *Browser) Close() error {
	b.pool.Close()
	b.connected = false
	if b.ctxCancel != nil {
		b.ctxCancel()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
	return nil
}
```

**Step 3: Add chromedp dependency**

```bash
cd /home/krolik/src/go-browser
go get github.com/chromedp/chromedp@latest
go mod tidy
```

**Step 4: Commit**

```bash
cd /home/krolik/src/go-browser
git add remote/ go.mod go.sum
git commit -m "feat: add remote CDP backend (browserless/lightpanda compatible)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 5: Lint + CI Setup

**Files:**
- Create: `.golangci.yml`
- Create: `Makefile`
- Create: `.pre-commit-config.yaml`

**Step 1: Write .golangci.yml**

```yaml
version: "2"
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - ineffassign
    - gosimple
    - revive
  settings:
    revive:
      rules:
        - name: exported
          arguments: [checkPrivateReceivers]
linters-settings: {}
```

**Step 2: Write Makefile**

```makefile
.PHONY: lint test test-integration

lint:
	golangci-lint run ./...

test:
	go test ./... -short -v -count=1

test-integration:
	go test ./... -v -count=1 -timeout 60s
```

**Step 3: Write .pre-commit-config.yaml**

```yaml
repos:
  - repo: https://github.com/golangci/golangci-lint
    rev: v2.1.6
    hooks:
      - id: golangci-lint-full
```

**Step 4: Run lint**

```bash
cd /home/krolik/src/go-browser && make lint
```

Expected: PASS (or fix any issues).

**Step 5: Commit**

```bash
cd /home/krolik/src/go-browser
git add .golangci.yml Makefile .pre-commit-config.yaml
git commit -m "chore: add lint config, Makefile, pre-commit hooks

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 6: GitHub Repo + Push

**Step 1: Create GitHub repo**

```bash
gh repo create anatolykoptev/go-browser --public --source /home/krolik/src/go-browser --push
```

**Step 2: Tag initial release**

```bash
cd /home/krolik/src/go-browser
git tag v0.1.0
git push origin v0.1.0
```

---

### Task 7: Migrate go-search

**Files:**
- Modify: `/home/krolik/src/go-search/internal/engine/browser.go` — replace with go-browser
- Modify: `/home/krolik/src/go-search/internal/engine/config.go` — init go-browser
- Modify: `/home/krolik/src/go-search/internal/engine/fetch.go` — use Browser interface
- Modify: `/home/krolik/src/go-search/go.mod` — add go-browser dep

**Step 1: Add go-browser dependency to go-search**

```bash
cd /home/krolik/src/go-search
go get github.com/anatolykoptev/go-browser@v0.1.0
```

**Step 2: Rewrite browser.go — replace chromedp globals with Browser field**

Replace the entire `browser.go` with:

```go
package engine

import (
	"context"
	"log/slog"

	browser "github.com/anatolykoptev/go-browser"
	"github.com/anatolykoptev/go-browser/rod"
	"github.com/anatolykoptev/go-browser/remote"
	"github.com/anatolykoptev/go-kit/env"
)

var pageBrowser browser.Browser

func initBrowser() {
	backend := env.Str("BROWSER_BACKEND", "remote")

	switch backend {
	case "rod":
		b, err := rod.New(
			func(o *rod.Options) { o.Concurrency = 3 },
			func(o *rod.Options) { o.RenderTimeout = browserRenderTimeout },
		)
		if err != nil {
			slog.Error("browser: rod launch failed", "err", err)
			return
		}
		pageBrowser = b

	default: // "remote"
		wsURL := env.Str("BROWSER_WS_URL", "")
		if wsURL == "" {
			slog.Info("browser: disabled (no BROWSER_WS_URL)")
			return
		}
		b, err := remote.New(
			remote.WithEndpoint(wsURL),
			func(o *remote.Options) { o.Concurrency = 3 },
			func(o *remote.Options) { o.RenderTimeout = browserRenderTimeout },
		)
		if err != nil {
			slog.Error("browser: remote connect failed", "err", err)
			return
		}
		pageBrowser = b
	}

	slog.Info("browser: ready", "backend", backend)
}

// BrowserAvailable reports whether a browser backend is connected.
func BrowserAvailable() bool {
	return pageBrowser != nil && pageBrowser.Available()
}

// fetchWithBrowser renders a page and returns outerHTML.
func fetchWithBrowser(ctx context.Context, url string) (string, error) {
	page, err := pageBrowser.Render(ctx, url)
	if err != nil {
		return "", err
	}
	return page.HTML, nil
}

func closeBrowser() {
	if pageBrowser != nil {
		pageBrowser.Close()
	}
}
```

**Step 3: Verify fetch.go needs no changes**

`fetch.go` already calls `BrowserAvailable()` and `fetchWithBrowser()` / `fetchWithBrowserExtract()` — these function signatures are preserved. No changes needed.

**Step 4: Remove chromedp dependency**

```bash
cd /home/krolik/src/go-search
go mod tidy
```

**Step 5: Build + test**

```bash
cd /home/krolik/src/go-search && go build ./...
```

**Step 6: Deploy and verify**

```bash
cd ~/deploy/krolik-server
docker compose build --no-cache go-search && docker compose up -d --no-deps --force-recreate go-search
curl http://127.0.0.1:8890/health
```

**Step 7: Commit**

```bash
cd /home/krolik/src/go-search
git add -A && git commit -m "refactor: migrate browser to go-browser library

Replace inline chromedp code with go-browser.Browser interface.
Supports both 'remote' (browserless) and 'rod' (in-process) backends
via BROWSER_BACKEND env var.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 8: Migrate go-wp

**Files:**
- Modify: `/home/krolik/src/go-wp/browser_init.go` — replace with go-browser
- Modify: `/home/krolik/src/go-wp/main.go` — update init call
- Modify: `/home/krolik/src/go-wp/go.mod` — add go-browser dep

**Step 1: Add go-browser dependency**

```bash
cd /home/krolik/src/go-wp
go get github.com/anatolykoptev/go-browser@v0.1.0
```

**Step 2: Rewrite browser_init.go**

Replace the entire file with:

```go
package main

import (
	"context"
	"log/slog"
	"time"

	browser "github.com/anatolykoptev/go-browser"
	"github.com/anatolykoptev/go-browser/rod"
	"github.com/anatolykoptev/go-browser/remote"
	"github.com/anatolykoptev/go-kit/env"
)

const (
	browserRenderTimeout = 25 * time.Second
	browserHydrationWait = 3 * time.Second
	browserConcurrency   = 2
)

var pageBrowser browser.Browser

func initBrowser() {
	backend := env.Str("BROWSER_BACKEND", "remote")

	switch backend {
	case "rod":
		b, err := rod.New(
			func(o *rod.Options) { o.Concurrency = browserConcurrency },
			func(o *rod.Options) { o.RenderTimeout = browserRenderTimeout },
			func(o *rod.Options) { o.HydrationWait = browserHydrationWait },
		)
		if err != nil {
			slog.Error("browser: rod launch failed", "err", err)
			return
		}
		pageBrowser = b

	default: // "remote"
		wsURL := env.Str("BROWSER_WS_URL", "")
		if wsURL == "" {
			slog.Info("browser: disabled (no BROWSER_WS_URL)")
			return
		}
		b, err := remote.New(
			remote.WithEndpoint(wsURL),
			func(o *remote.Options) { o.Concurrency = browserConcurrency },
			func(o *remote.Options) { o.RenderTimeout = browserRenderTimeout },
			func(o *remote.Options) { o.HydrationWait = browserHydrationWait },
		)
		if err != nil {
			slog.Error("browser: remote connect failed", "err", err)
			return
		}
		pageBrowser = b
	}

	slog.Info("browser: ready", "backend", backend)
}

func browserFetch(ctx context.Context, url string) (string, error) {
	if pageBrowser == nil || !pageBrowser.Available() {
		return "", browser.ErrUnavailable
	}
	page, err := pageBrowser.Render(ctx, url)
	if err != nil {
		return "", err
	}
	return page.HTML, nil
}

func closeBrowser() {
	if pageBrowser != nil {
		pageBrowser.Close()
	}
}
```

**Step 3: main.go — no changes needed**

`main.go` already calls `initBrowser()`, `closeBrowser()`, and checks `browserCtx != nil`.
Update the nil check to use `pageBrowser`:

In `main.go` change:
```go
if browserCtx != nil {
    deps.BrowserFetch = browserFetch
}
```
to:
```go
if pageBrowser != nil && pageBrowser.Available() {
    deps.BrowserFetch = browserFetch
}
```

**Step 4: Remove chromedp dependency, vendor**

```bash
cd /home/krolik/src/go-wp
go mod tidy
go mod vendor
```

**Step 5: Build**

```bash
cd /home/krolik/src/go-wp && go build -mod=vendor ./...
```

**Step 6: Deploy and verify**

```bash
cd ~/deploy/krolik-server
docker compose build --no-cache go-wp && docker compose up -d --no-deps --force-recreate go-wp
curl http://127.0.0.1:8894/health
```

**Step 7: Commit**

```bash
cd /home/krolik/src/go-wp
git add -A && git commit -m "refactor: migrate browser to go-browser library

Replace inline chromedp code with go-browser.Browser interface.
Supports 'remote' and 'rod' backends via BROWSER_BACKEND env var.
Preserves browserFetch() signature for deps.BrowserFetch compatibility.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 9: Switch to Rod + Remove Browserless Container

**Prerequisite:** Tasks 7 + 8 deployed and verified with `remote` backend.

**Step 1: Update docker-compose.yml — add Chromium to go-search and go-wp**

In `docker-compose.yml`, for both `go-search` and `go-wp`, change env:

```yaml
# go-search
- BROWSER_BACKEND=rod
# Remove: - BROWSER_WS_URL=ws://browserless:3000

# go-wp
- BROWSER_BACKEND=rod
# Remove: - BROWSER_WS_URL=ws://browserless:3000
```

**Step 2: Update Dockerfiles — install Chromium**

Add to go-search and go-wp Dockerfiles:

```dockerfile
# Chromium for Rod headless browser
RUN apk add --no-cache chromium
ENV BROWSER_BIN=/usr/bin/chromium-browser
```

**Step 3: Remove browserless service from docker-compose.yml**

Delete the entire `browserless:` service block and remove `browserless` from `depends_on` in go-search and go-wp.

**Step 4: Deploy**

```bash
cd ~/deploy/krolik-server
docker compose build --no-cache go-search go-wp
docker compose up -d --no-deps --force-recreate go-search go-wp
docker compose stop browserless && docker compose rm -f browserless
```

**Step 5: Verify**

```bash
curl http://127.0.0.1:8890/health
curl http://127.0.0.1:8894/health
docker compose logs go-search 2>&1 | grep "browser:" | tail -3
docker compose logs go-wp 2>&1 | grep "browser:" | tail -3
```

Expected: `browser: ready backend=rod`

**Step 6: Commit docker-compose changes**

```bash
cd ~/deploy/krolik-server
git add docker-compose.yml
git commit -m "feat: switch to Rod browser, remove browserless container

go-search and go-wp now run in-process Chromium via go-browser/rod.
Frees ~384MB RAM from the browserless container.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 10: README + Docs

**Files:**
- Create: `/home/krolik/src/go-browser/README.md`

**Step 1: Write README.md**

```markdown
# go-browser

Shared Go library for headless browser automation with pluggable backends.

## Backends

| Backend | Import | Use Case |
|---------|--------|----------|
| Rod | `go-browser/rod` | In-process Chromium (default) |
| Remote | `go-browser/remote` | External CDP endpoint (browserless, Lightpanda) |

## Usage

```go
import (
    "github.com/anatolykoptev/go-browser/rod"
)

b, _ := rod.New()
defer b.Close()

page, _ := b.Render(ctx, "https://example.com")
fmt.Println(page.Title, len(page.HTML))
```

## With Proxy

```go
pool, _ := proxypool.NewWebshare(apiKey)
b, _ := rod.New(rod.WithProxyPool(pool))
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/ROADMAP.md](docs/ROADMAP.md).
```

**Step 2: Commit + push + tag**

```bash
cd /home/krolik/src/go-browser
git add README.md
git commit -m "docs: add README

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
git tag v0.2.0
git push origin main v0.2.0
```
