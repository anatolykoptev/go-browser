# go-browser

[![Go Reference](https://pkg.go.dev/badge/github.com/anatolykoptev/go-browser.svg)](https://pkg.go.dev/github.com/anatolykoptev/go-browser)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Headless browser library for Go with pluggable backends, crash recovery, proxy rotation, and resource blocking.

## Why

Headless browser rendering in Go usually means either:

- **chromedp** — tied to a running Chrome, no crash recovery, manual pool management
- **Rod** — better API, but still raw — you wire up proxy, resource blocking, retry logic yourself

go-browser wraps both behind a single `Browser` interface, adds production features (crash recovery, resource blocking, proxy pool), and lets you swap backends without touching application code.

## Install

```
go get github.com/anatolykoptev/go-browser@v0.3.1
```

Requires Go 1.26+.

## Quick Start

### Rod (in-process Chromium)

```go
package main

import (
    "context"
    "fmt"
    "log"

    rodbackend "github.com/anatolykoptev/go-browser/rod"
)

func main() {
    b, err := rodbackend.New(rodbackend.WithHeadless(true))
    if err != nil {
        log.Fatal(err)
    }
    defer b.Close()

    page, err := b.Render(context.Background(), "https://example.com")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%s (%d bytes)\n", page.Title, len(page.HTML))
}
```

Rod auto-downloads Chromium on first run. Pass `WithBin("/usr/bin/chromium-browser")` to use a system binary.

### Remote (external CDP endpoint)

```go
import "github.com/anatolykoptev/go-browser/remote"

b, err := remote.New(remote.WithEndpoint("ws://browserless:3000"))
if err != nil {
    log.Fatal(err)
}
defer b.Close()

page, err := b.Render(ctx, "https://example.com")
```

Compatible with Browserless, Lightpanda, or any CDP WebSocket endpoint.

## Interface

Both backends implement the same interface:

```go
type Browser interface {
    Render(ctx context.Context, url string) (*Page, error)
    Available() bool
    Close() error
}

type Page struct {
    URL    string // final URL after redirects
    HTML   string // rendered outerHTML
    Title  string // page title
    Status int    // HTTP status (0 if unknown)
}
```

## Proxy

```go
pool, _ := proxypool.NewWebshare(apiKey)
b, _ := rodbackend.New(rodbackend.WithProxyPool(pool))
```

The proxy is set at Chromium launch via `--proxy-server`. All traffic from all tabs goes through the proxy. Accepts any `go-stealth.ProxyPoolProvider` (Webshare, static list, healthy pool wrapper).

## Resource Blocking

Skip images, fonts, CSS, and media to speed up renders:

```go
b, _ := rodbackend.New(
    rodbackend.WithBlockResources(
        rodbackend.ResourceImage,
        rodbackend.ResourceFont,
        rodbackend.ResourceStylesheet,
    ),
)
```

Uses Rod's `HijackRequests` to abort matching requests before they hit the network. Available types: `ResourceImage`, `ResourceFont`, `ResourceStylesheet`, `ResourceMedia`.

## Crash Recovery

If Chromium dies mid-render (OOM, segfault, zombie), the Rod backend:

1. Detects the dead connection (websocket close, broken pipe, EOF)
2. Kills the zombie process and launches fresh Chromium with same options
3. Retries the failed render once

No container restart needed. Verified by integration test that `SIGKILL`s Chromium and confirms recovery.

## Options

### Rod

| Option | Default | Description |
|--------|---------|-------------|
| `WithBin(path)` | auto-download | Chromium binary path |
| `WithHeadless(bool)` | `true` | Headless mode |
| `WithProxyPool(pool)` | none | Proxy rotation via go-stealth |
| `WithBlockResources(...)` | none | Resource types to abort |

### Common (both backends)

| Field | Default | Description |
|-------|---------|-------------|
| `Concurrency` | `3` | Max concurrent page renders |
| `RenderTimeout` | `20s` | Per-render deadline |
| `HydrationWait` | `2s` | Wait for DOM to stabilize after body ready |
| `UserAgent` | browser default | Override User-Agent header |

Set common options via functional option or direct field:

```go
rodbackend.New(
    func(o *rodbackend.Options) { o.Concurrency = 5 },
    func(o *rodbackend.Options) { o.RenderTimeout = 30 * time.Second },
)
```

### Remote

| Option | Default | Description |
|--------|---------|-------------|
| `WithEndpoint(url)` | none | CDP WebSocket URL |

## Errors

Sentinel errors for `errors.Is` matching:

```go
browser.ErrNavigate    // DNS, TLS, HTTP failure, or page creation error
browser.ErrTimeout     // render exceeded deadline or DOM didn't stabilize
browser.ErrUnavailable // backend not connected or browser binary not found
```

## Architecture

```
github.com/anatolykoptev/go-browser
├── browser.go       Browser interface, Page struct
├── options.go       Common options (Concurrency, RenderTimeout, HydrationWait)
├── errors.go        Sentinel errors
├── pool.go          Channel-based semaphore with context cancellation
│
├── rod/             In-process Chromium backend
│   ├── rod.go       New, Render (with retry wrapper), Close
│   ├── options.go   Rod-specific options (Bin, ProxyPool, BlockResources)
│   ├── restart.go   isConnectionError detection + browser restart
│   └── hijack.go    Request interception for resource blocking
│
└── remote/          External CDP endpoint backend
    ├── remote.go    New, Render, Close (via chromedp)
    └── options.go   Remote-specific options (Endpoint)
```

## Stealth Self-Test (`/selftest`)

The HTTP server exposes a `/selftest` endpoint that runs the live CloakBrowser instance
against public antibot probe pages and returns a structured JSON trust report.

### Endpoint

```
GET /selftest?target=all&profile=mac_chrome145&screenshot=1
```

| Parameter    | Values                                       | Default         |
|--------------|----------------------------------------------|-----------------|
| `target`     | `creepjs`, `sannysoft`, `rebrowser`, `botd`, `webrtc_leak`, `canvas`, `all` | all targets     |
| `profile`    | any profile name in `stealth/profiles/`      | `mac_chrome145` |
| `screenshot` | `1` to save PNGs to `/tmp/selftest/`         | off             |

Multiple targets: `?target=creepjs,sannysoft`

### Response

```json
{
  "profile": "mac_chrome145",
  "started_at": "2026-04-09T23:00:00Z",
  "results": [
    {
      "target": "creepjs",
      "url": "https://abrahamjuliot.github.io/creepjs/",
      "duration_ms": 12340,
      "ok": true,
      "trust_score": 95.5,
      "lies": [],
      "sections": {
        "fonts":  {"hash": "a1b2c3", "platformClassifier": "Apple"},
        "webrtc": {"publicIp": "185.x.x.x", "localIps": []},
        "audio":  {"hash": "196.239479"},
        "voices": {"count": 34, "hash": "e5f6g7"},
        "ua":     {"brands": [...], "platform": "macOS"}
      },
      "screenshot_path": "/tmp/selftest/creepjs-20260409T230000.png"
    }
  ],
  "summary": {
    "total": 6,
    "passed": 5,
    "failed": 1,
    "overall_trust": 92.3
  }
}
```

### Supported Targets

| Key           | URL                                              | What it measures                      |
|---------------|--------------------------------------------------|---------------------------------------|
| `creepjs`     | abrahamjuliot.github.io/creepjs/                 | Trust score, lies, section hashes     |
| `sannysoft`   | bot.sannysoft.com                                | Pass/fail checklist                   |
| `rebrowser`   | bot-detector.rebrowser.net                       | `window.botDetectorResults` checks    |
| `botd`        | fingerprintjs.github.io/BotD/main/               | FingerprintJS BotD verdict            |
| `webrtc_leak` | browserleaks.com/webrtc                          | RFC1918 IP leak detection             |
| `canvas`      | browserleaks.com/canvas                          | Canvas fingerprint hash               |

### Quick curl

```bash
# Single target
curl "http://localhost:8901/selftest?target=sannysoft" | jq .

# All targets + screenshots
curl "http://localhost:8901/selftest?target=all&screenshot=1" | jq '.summary'

# Specific profile
curl "http://localhost:8901/selftest?target=creepjs&profile=win_chrome145" | jq '.results[0].trust_score'
```

Per-target errors are embedded in `results[i].ok=false, error:"..."` — the endpoint
itself only returns 5xx if the browser is unavailable.

## Testing

```bash
# Unit tests (no Chromium needed)
go test ./... -race -count=1 -short

# Integration tests (requires Chromium — Rod auto-downloads or set BROWSER_BIN)
go test ./rod -race -count=1 -v -timeout 120s
```

Integration tests use `net/http/httptest` — no external network calls. Tests cover:

- **BasicRender** — end-to-end render of a local HTML page
- **ConcurrentRenders** — 5 goroutines through a pool of 2
- **ResourceBlocking** — control (`title="loaded"`) vs blocked (`title="blocked"`)
- **CrashRecovery** — SIGKILL Chromium, verify auto-restart and re-render

## Production Usage

Used in [go-search](https://github.com/anatolykoptev/go-search) (fallback for JS-heavy pages) and [go-wp](https://github.com/anatolykoptev/go-wp) (WordPress content fetching). Both run Rod backend with proxy pool and resource blocking in Docker containers.

## License

[MIT](LICENSE)
