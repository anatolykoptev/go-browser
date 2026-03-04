# go-browser

[![Go Reference](https://pkg.go.dev/badge/github.com/anatolykoptev/go-browser.svg)](https://pkg.go.dev/github.com/anatolykoptev/go-browser)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Pluggable headless browser library for Go with crash recovery, proxy rotation, and resource blocking.

## Features

- **Pluggable backends** — Rod (in-process Chromium) or Remote (external CDP endpoint)
- **Crash recovery** — automatic Chromium restart on connection failure
- **Proxy integration** — rotating proxy pool via [go-stealth](https://github.com/anatolykoptev/go-stealth)
- **Resource blocking** — skip images, fonts, CSS, media for faster renders
- **Concurrency control** — semaphore-based page pool
- **Configurable timeouts** — render timeout, DOM hydration wait

## Install

```bash
go get github.com/anatolykoptev/go-browser@latest
```

## Quick Start — Rod

```go
import rodbackend "github.com/anatolykoptev/go-browser/rod"

b, err := rodbackend.New()
if err != nil {
    log.Fatal(err)
}
defer b.Close()

page, err := b.Render(context.Background(), "https://example.com")
fmt.Println(page.Title, len(page.HTML))
```

## Quick Start — Remote

```go
import "github.com/anatolykoptev/go-browser/remote"

b, err := remote.New(remote.WithEndpoint("ws://browserless:3000"))
if err != nil {
    log.Fatal(err)
}
defer b.Close()

page, err := b.Render(ctx, "https://example.com")
```

## Proxy Integration

```go
pool, _ := proxypool.NewWebshare(apiKey)
b, _ := rodbackend.New(rodbackend.WithProxyPool(pool))
```

The proxy is set at Chromium launch time. All requests from all tabs go through the proxy.

## Resource Blocking

Block unnecessary resources to speed up renders and save bandwidth:

```go
b, _ := rodbackend.New(
    rodbackend.WithBlockResources(
        rodbackend.ResourceImage,
        rodbackend.ResourceFont,
        rodbackend.ResourceStylesheet,
    ),
)
```

Available resource types: `ResourceImage`, `ResourceFont`, `ResourceStylesheet`, `ResourceMedia`.

## Crash Recovery

If Chromium dies mid-render (OOM, segfault, zombie process), the Rod backend detects the broken connection and automatically:

1. Closes the dead browser process
2. Launches a fresh Chromium with the same options
3. Retries the render once

No manual intervention or container restart required.

## Options Reference

| Option | Default | Description |
|--------|---------|-------------|
| `WithBin(path)` | auto-download | Custom Chromium binary path |
| `WithHeadless(bool)` | `true` | Headless mode |
| `WithProxyPool(pool)` | none | Rotating proxy via go-stealth |
| `WithBlockResources(types...)` | none | Block resource types |
| `Concurrency` | `3` | Max concurrent pages |
| `RenderTimeout` | `15s` | Per-render timeout |
| `HydrationWait` | `1s` | DOM stability wait |

## Error Handling

Sentinel errors for matching:

- `browser.ErrNavigate` — navigation or page creation failed
- `browser.ErrTimeout` — render exceeded timeout or DOM didn't stabilize
- `browser.ErrUnavailable` — backend not connected

## Architecture

```
browser.Browser (interface)
├── rod.Browser      — in-process Chromium via go-rod
│   ├── restart.go   — crash detection + auto-restart
│   └── hijack.go    — request interception (resource blocking)
└── remote.Browser   — external CDP endpoint via chromedp
```

Both backends share the same `browser.Options` (concurrency, timeouts) and return `*browser.Page`.

## License

MIT
