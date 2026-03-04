# go-browser Architecture

Shared Go library for headless browser automation. Replaces per-service `chromedp` boilerplate
and the external `browserless` Docker container with a unified, multi-backend interface.

## Problem

- **go-wp** and **go-search** both have ~100-line `browser_init.go` with identical chromedp logic
- Both connect to a shared `browserless` container (384MB RAM, separate process)
- Duplication: remote allocator setup, concurrency semaphore, render timeout, hydration wait
- No fallback: if browserless is down, browser features silently degrade to nothing

## Solution

One library with pluggable backends behind a `Browser` interface.

## Interface

```go
package browser

type Browser interface {
    // Render navigates to URL, waits for JS, returns rendered HTML.
    Render(ctx context.Context, url string) (*Page, error)

    // Close shuts down browser and releases resources.
    Close() error

    // Available reports whether the browser backend is connected and usable.
    Available() bool
}

type Page struct {
    URL     string            // Final URL after redirects
    HTML    string            // Rendered outerHTML
    Title   string            // Page title
    Status  int               // HTTP status code
    Headers map[string]string // Response headers
}
```

## Backends

### 1. Rod (primary)

In-process Chromium via Chrome DevTools Protocol.

```
go-browser/rod/
├── rod.go          -- Browser impl: launch, pool, render
└── options.go      -- WithProxy, WithHeadless, WithBin
```

- Manages Chromium lifecycle (auto-download or custom binary)
- Built-in page pool with concurrency limit
- Proxy integration via go-stealth ProxyPool
- Replaces `browserless` container entirely

### 2. Lightpanda (lightweight)

External CDP-compatible process. 9x less memory than Chrome.

```
go-browser/lightpanda/
├── lightpanda.go   -- Browser impl: connect via CDP ws://
└── options.go      -- WithEndpoint, WithCloudToken
```

- Connects via `chromedp.NewRemoteAllocator` (same CDP protocol)
- Local binary or Lightpanda Cloud endpoint
- No screenshots/PDF (DOM + JS only)
- Ideal for high-volume fetch where JS execution needed

### 3. Remote (compatibility)

Connect to any CDP-compatible endpoint (browserless, Chrome, Lightpanda).

```
go-browser/remote/
├── remote.go       -- Browser impl: connect to ws:// endpoint
└── options.go      -- WithEndpoint, WithMaxTabs
```

- Drop-in replacement for current go-wp/go-search chromedp code
- Backward compatible with existing `BROWSER_WS_URL` config
- Migration path: remote → rod → lightpanda

## Package Structure

```
go-browser/                        (~800 LOC target)
├── browser.go                     -- Browser + Page interfaces
├── options.go                     -- Common options (timeout, concurrency, user-agent)
├── pool.go                        -- Page pool (semaphore + reuse)
├── rod/
│   ├── rod.go                     -- Rod backend
│   └── options.go                 -- Rod-specific options
├── lightpanda/
│   ├── lightpanda.go              -- Lightpanda backend
│   └── options.go                 -- Lightpanda-specific options
├── remote/
│   ├── remote.go                  -- Generic CDP remote backend
│   └── options.go                 -- Remote-specific options
└── docs/
    ├── ARCHITECTURE.md            -- This file
    └── ROADMAP.md                 -- Implementation phases
```

## Integration with go-stealth

go-browser imports go-stealth for proxy rotation, not the other way around.

```
go-stealth (HTTP client, TLS fingerprinting, proxy pool)
     ↑
go-browser (headless browser, JS rendering)
     ↑
go-enriche / go-wp / go-search (consumers)
```

Proxy flow:
```go
pool, _ := proxypool.NewWebshare(apiKey)

b, _ := rod.New(
    browser.WithConcurrency(3),
    browser.WithTimeout(20 * time.Second),
    rod.WithProxyPool(pool),  // go-stealth ProxyPool
)

page, _ := b.Render(ctx, "https://example.com")
fmt.Println(page.HTML)
```

## Concurrency Model

Each backend manages a page pool internally:

```
Browser
  └── Pool (semaphore, default 3)
       ├── Page 1 (in use)
       ├── Page 2 (in use)
       └── Page 3 (idle, reusable)
```

- Semaphore limits concurrent renders (configurable via `WithConcurrency`)
- Pages are reused across requests (navigate, not create)
- Context cancellation aborts render and releases slot

## Current Consumers

| Service | Current Code | Browser Use | Migration |
|---------|-------------|-------------|-----------|
| go-wp | `browser_init.go` (87 LOC) | Yandex Maps org pages (SPA) | `rod` backend |
| go-search | `browser.go` (101 LOC) | Fallback for JS-heavy pages | `rod` or `lightpanda` |
| go-enriche | None | Future: JS-rendered content | `lightpanda` (lightweight) |

## Configuration

Environment variables (backward compatible):

| Var | Description | Default |
|-----|-------------|---------|
| `BROWSER_BACKEND` | `rod`, `lightpanda`, `remote` | `rod` |
| `BROWSER_WS_URL` | Remote CDP endpoint (remote/lightpanda) | — |
| `BROWSER_CONCURRENCY` | Max concurrent renders | `3` |
| `BROWSER_TIMEOUT` | Render timeout | `20s` |
| `BROWSER_BIN` | Custom browser binary path | auto-detect |

## Error Handling

- `ErrUnavailable` — backend not connected / binary not found
- `ErrTimeout` — render exceeded deadline
- `ErrNavigate` — page load failed (DNS, TLS, HTTP error)
- All errors are wrapped with backend name for debugging
- `Available()` allows graceful degradation (same pattern as current code)
