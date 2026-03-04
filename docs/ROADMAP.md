# go-browser Roadmap

## Phase 0: Bootstrap (v0.1.0)

**Goal:** Interface + Remote backend. Drop-in replacement for existing chromedp code.

- [x] Define `Browser`, `Page` interfaces
- [x] Common options: `WithConcurrency`, `WithTimeout`, `WithUserAgent`
- [x] Page pool with semaphore
- [x] `remote/` backend — connect to `ws://` CDP endpoint (chromedp under the hood)
- [x] Migrate go-search `browser.go` → `browser.New(remote.WithEndpoint(wsURL))`
- [x] Migrate go-wp `browser_init.go` → same pattern
- [x] Tests: pool concurrency, timeout, unavailable graceful degradation
- [x] Delete `browserless` container from docker-compose (after Rod in Phase 1)

**Result:** Same behavior, shared code, one dependency instead of two copy-pasted files.

## Phase 1: Rod Backend (v0.2.0)

**Goal:** In-process Chromium. Eliminate `browserless` container.

- [x] `rod/` backend — launch + manage Chromium via Rod
- [x] Auto-download Chromium binary on first run
- [x] Proxy integration via `go-stealth.ProxyPool`
- [x] Page pool with Rod's built-in browser.EachEvent
- [x] Docker: add Chromium to service containers that need it (go-search, go-wp)
- [x] Remove `browserless` service from docker-compose
- [x] Benchmark: memory per tab, render latency vs browserless
- [x] Tests: proxy rotation, concurrent renders, crash recovery

**Result:** No more external browserless container. ~384MB RAM freed.

## Phase 2: Lightpanda Backend (v0.3.0)

**Goal:** Lightweight alternative for high-volume JS rendering.

- [ ] `lightpanda/` backend — connect to Lightpanda CDP server
- [ ] Lightpanda binary management (download, health check)
- [ ] Docker: Lightpanda sidecar or embedded binary
- [ ] Benchmark: memory/latency vs Rod for typical enrichment pages
- [ ] Fallback: Lightpanda → Rod when Lightpanda can't render (missing Web APIs)
- [ ] go-enriche integration: `WithBrowser(browser.Browser)` option

**Depends on:** Lightpanda stability (currently beta). Monitor Web API coverage.

**Result:** 9x less memory for JS rendering in enrichment pipeline.

## Phase 3: Advanced Features (v0.4.0)

**Goal:** Production hardening + advanced capabilities.

- [x] Request interception (block images/fonts/analytics for faster renders)
- [ ] Cookie injection (authenticated scraping)
- [ ] Screenshot support (Rod backend only)
- [ ] Custom JS injection (wait conditions, data extraction scripts)
- [ ] Metrics: render count, latency histogram, error rate (callback hooks)
- [x] Health monitoring: auto-restart crashed browser process

## Phase 4: Anti-Detection (v0.5.0)

**Goal:** Stealth browser sessions that pass bot detection.

- [ ] Browser fingerprint rotation (viewport, timezone, locale, WebGL)
- [ ] Human-like behavior (random delays, scroll patterns)
- [ ] Integration with go-stealth profiles (match TLS fingerprint to browser UA)
- [ ] Cloudflare/Turnstile bypass testing
- [ ] Per-session proxy binding (proxy ↔ fingerprint consistency)

## Future: Kalamari (monitoring)

Pure Rust headless browser, 10MB binary, no Chromium. Currently v0.1, 3 GitHub stars.

**Watch for:**
- JS execution support (currently DOM-only)
- Stability improvements
- Community adoption (stars, contributors)
- Go FFI or sidecar integration feasibility

**When ready:** Add `kalamari/` backend with same `Browser` interface.

## Migration Plan

### Step 1: go-search (lowest risk)

Browser is optional fallback (thin content < 200 chars). Safe to swap.

```go
// Before (browser.go, 101 lines):
chromedp.NewRemoteAllocator(ctx, wsURL)

// After:
b, _ := remote.New(remote.WithEndpoint(wsURL))
page, _ := b.Render(ctx, url)
```

### Step 2: go-wp (medium risk)

Browser used for Yandex Maps org data. Test with known place URLs.

```go
// Before (browser_init.go, 87 lines):
chromedp.NewRemoteAllocator(ctx, wsURL)

// After:
b, _ := rod.New(
    browser.WithConcurrency(2),
    browser.WithTimeout(25 * time.Second),
    rod.WithProxyPool(pool),
)
deps.BrowserFetch = func(ctx context.Context, url string) (string, error) {
    page, err := b.Render(ctx, url)
    if err != nil { return "", err }
    return page.HTML, nil
}
```

### Step 3: go-enriche (new integration)

Add optional `WithBrowser` to enriche for JS-rendered content extraction.

```go
b, _ := lightpanda.New(lightpanda.WithEndpoint("ws://127.0.0.1:9222"))
e := enriche.New(
    enriche.WithStealth(stealthClient),
    enriche.WithBrowser(b),  // new option
)
// Enricher tries HTTP first, falls back to browser for thin content
```

## Non-Goals

- **Full test automation framework** — not Selenium/Playwright replacement
- **Screenshot service** — incidental feature of Rod, not primary purpose
- **PDF generation** — use dedicated tools (wkhtmltopdf, etc.)
- **Multi-browser support** — Chromium-family only (Rod + Lightpanda)
