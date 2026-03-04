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
    "context"
    rodbackend "github.com/anatolykoptev/go-browser/rod"
)

b, _ := rodbackend.New()
defer b.Close()

page, _ := b.Render(context.Background(), "https://example.com")
fmt.Println(page.Title, len(page.HTML))
```

## Remote CDP

```go
import "github.com/anatolykoptev/go-browser/remote"

b, _ := remote.New(remote.WithEndpoint("ws://browserless:3000"))
defer b.Close()

page, _ := b.Render(ctx, "https://example.com")
```

## With Proxy

```go
pool, _ := proxypool.NewWebshare(apiKey)
b, _ := rodbackend.New(rodbackend.WithProxyPool(pool))
```

## Options

```go
rodbackend.New(
    rodbackend.WithBin("/usr/bin/chromium-browser"),
    rodbackend.WithHeadless(true),
    func(o *rodbackend.Options) { o.Concurrency = 3 },
    func(o *rodbackend.Options) { o.RenderTimeout = 20 * time.Second },
    func(o *rodbackend.Options) { o.HydrationWait = 2 * time.Second },
)
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/ROADMAP.md](docs/ROADMAP.md).
