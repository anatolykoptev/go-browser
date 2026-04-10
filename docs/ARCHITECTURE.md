# Browser Architecture — Two-Tier System

## Overview

Two independent browser services working together:

```
                    ┌─────────────────────────────┐
                    │         Consumers            │
                    │  go-search, go-wp, go-social │
                    │  go-enriche, go-code         │
                    └─────────┬───────────────────┘
                              │
              ┌───────────────┼───────────────┐
              │                               │
     ┌────────▼────────┐           ┌──────────▼─────────┐
     │  ox-browser      │           │  go-browser         │
     │  :8901 (Rust)    │           │  :8906 (Go)         │
     │                  │           │                     │
     │  HTTP stealth    │           │  Chrome automation  │
     │  wreq+BoringSSL  │           │  go-rod + CloakBrowser │
     │  16-1400ms/req   │           │  300-6300ms/req     │
     └──────────────────┘           └─────────┬───────────┘
                                              │ CDP (ws://)
                                    ┌─────────▼───────────┐
                                    │  CloakBrowser        │
                                    │  :9222 (C++ patched  │
                                    │  Chromium 145)       │
                                    │  Headed via Xvfb     │
                                    └─────────────────────┘
```

## When to Use What

| Need | Use | Why |
|------|-----|-----|
| Fetch HTML (no JS) | ox-browser `/fetch` | 16ms, lightweight |
| Extract text/readability | ox-browser `/read` | Built-in trafilatura |
| CF-protected page | ox-browser `/fetch` | wreq TLS bypass, 1.4s |
| JS-rendered SPA | go-browser `/render` | Real Chrome renders JS |
| Form interaction (login, search) | go-browser `/chrome/interact` | CDP actions |
| Screenshot | go-browser `/chrome/interact` | Real rendering |
| Technology detection | ox-browser `/analyze` | Wappalyzer DB |
| Security scan | ox-browser `/security` | Header analysis |
| Crawling | ox-browser `/crawl` | BFS with rate limits |
| Media download | ox-browser `/media/download` | yt-dlp + ffmpeg |

## Tier 1: ox-browser (Rust HTTP)

**Port**: 8901 | **RAM**: ~50MB | **Speed**: 16-1400ms

Stealth HTTP client without a real browser. Uses wreq (BoringSSL) for Chrome-identical TLS/HTTP2 fingerprints.

### Key Features
- **TLS fingerprinting**: JA4, Akamai H2 — identical to real Chrome via BoringSSL
- **16 browser profiles**: Chrome/Firefox/Safari/Edge × Win/Mac/Linux/Mobile
- **Proxy pool**: Webshare (215K residential), round-robin, health tracking
- **CF detection**: JsChallenge/Turnstile/Block → auto-retry with different proxy
- **Middleware chain**: logging → rate_limit → retry → cloudflare → client_hints → wreq
- **MCP server**: 11 tools via rmcp (Streamable HTTP)

### Endpoints
| Endpoint | Purpose |
|----------|---------|
| `/fetch` | Raw HTML fetch with stealth headers |
| `/read` | Fetch + readability extraction (trafilatura) |
| `/fetch-smart` | /read alias |
| `/analyze` | Technology detection (Wappalyzer) |
| `/security` | Security header scan |
| `/crawl` | BFS web crawler |
| `/solve_cf` | Cloudflare challenge solver |
| `/images/search` | Image search (5 engines) |
| `/images/reverse` | Reverse image search |
| `/media/download` | Media download (YouTube, etc.) |
| `/site-audit` | Full site SEO/security audit |

### Architecture Detail
See `ox-browser/docs/ARCHITECTURE.md` for full crate dependency graph, middleware chain, proxy pool, and CF bypass architecture.

## Tier 2: go-browser (Chrome Automation)

**Port**: 8906 | **RAM**: ~300MB (with CloakBrowser) | **Speed**: 300-6300ms

Real Chrome browser (CloakBrowser) controlled via CDP (go-rod). Full JS rendering, form interaction, screenshots.

### Key Features
- **Headed Chrome** via Xvfb (not headless) — eliminates 8+ detection vectors natively
- **CloakBrowser**: 33 C++ patches (canvas, WebGL, audio, fonts, locale, CDP input)
- **Stealth JS**: 6 modules, Function.prototype.toString masking, Worker Blob/data: URL proxy
- **17 action types**: click, type_text, wait_for, evaluate, screenshot, scroll, warmup, etc.
- **Humanized interaction**: Bezier mouse paths, keyDown/char/keyUp typing, idle drift
- **Session pool**: Persistent browser contexts with cookies
- **Proxy auth**: CDP Fetch.authRequired handler (Webshare support)

### Endpoints
| Endpoint | Purpose |
|----------|---------|
| `/render` | Navigate + wait load + return HTML |
| `/chrome/interact` | Execute action sequence (click, type, eval, etc.) |
| `/solve` | Cloudflare challenge solver (via Chrome) |
| `/health` | Health check |

### CloakBrowser Configuration
```yaml
# Headed Chrome with Xvfb virtual display
command: >
  bash -c "dbus-daemon --system --fork;
  dbus-daemon --session --fork;
  Xvfb :99 -screen 0 1440x900x24 &
  chrome --use-gl=angle --use-angle=swiftshader --enable-webgl
  --window-size=1440,900 --fingerprint-platform=macos
  --timezone=America/Los_Angeles"
```

### Stealth System
See `go-browser/docs/STEALTH-SPEC.md` for full detection vector checklist.
See `go-browser/docs/STEALTH-ROADMAP.md` for planned improvements.
See `go-browser/docs/RESEARCH-LOG.md` for investigation history.

### Four-Layer Stealth Stack
```
Layer 4: go-wowa auto-bypass   (protection profiles, active evasion JS, canvas noise)
Layer 3: (removed — stealth_complement.js is the sole JS layer)
Layer 2: stealth_complement.js (6 modules, toString masking, Worker proxy)
Layer 1: CloakBrowser C++      (33 chromium patches)
```

Layer 4 is in go-wowa (not go-browser) — it wraps `RunInteract()` with detection + bypass.
See `go-wowa/docs/ARCHITECTURE.md` for auto-bypass pipeline details.

### Action Types
| Action | Description |
|--------|-------------|
| `click` | Click element (humanized: Bezier path + CDP mousePressed) |
| `type_text` | Type text (humanized: keyDown/char/keyUp per character) |
| `wait_for` | Wait for CSS selector |
| `evaluate` | Execute JS in page context |
| `eval_on_new_document` | Inject JS before page load |
| `navigate` | Navigate to URL |
| `screenshot` | Capture page screenshot (base64 PNG) |
| `snapshot` | Accessibility tree snapshot |
| `press` | Press keyboard key |
| `sleep` | Wait N milliseconds |
| `scroll` | Scroll element or page |
| `warmup` | Generate random mouse/scroll events (isTrusted:true) |
| `set_cookies` | Set cookies before navigation |
| `get_cookies` | Extract all page cookies |
| `get_logs` | Get network + console logs |
| `handle_dialog` | Accept/dismiss dialog |
| `hover` | Hover over element (humanized) |
| `go_back` | Navigate back |

## Performance Comparison

Tested 2026-03-28:

| Site | ox-browser (HTTP) | go-browser (Chrome) | Raw curl |
|------|:---:|:---:|:---:|
| HackerNews (simple) | 212ms ✅ | 322ms ✅ | 182ms ✅ |
| Amazon (CF protected) | 1,415ms ✅ | 6,299ms ✅ | 268ms ❌ blocked |
| bot.sannysoft.com | N/A | 56/56 tests pass | N/A |

## Anti-Bot Test Results (go-browser, 2026-03-28)

| Site | Status |
|------|--------|
| Amazon | ✅ 235 links, 19K chars |
| Booking.com | ✅ 104 links, 12K chars |
| StackOverflow | ✅ 67 links |
| GitHub Trending | ✅ 141 links |
| Авито | ✅ 53 links |
| 2ГИС | ✅ 12 links |
| Reddit | ✅ 190K chars |
| Хабр | ✅ 310K chars |
| Medium | ✅ 428K chars |
| Twitter (profile) | ✅ 318K chars |
| bot.sannysoft.com | ✅ **56/56 all pass** |

## Deployment

Both services in `~/deploy/krolik-server/compose/search.yml`:

```bash
# ox-browser (Rust HTTP)
docker compose build ox-browser && docker compose up -d --no-deps --force-recreate ox-browser

# go-browser + CloakBrowser (Chrome)
docker compose build go-browser && docker compose up -d --no-deps --force-recreate go-browser cloakbrowser
```

## Source Code

| Component | Path | Language | Tests |
|-----------|------|----------|-------|
| ox-browser | `~/src/ox-browser/` | Rust | 187 |
| go-browser | `~/src/go-browser/` | Go | 111 |
| go-stealth | `~/src/go-stealth/` | Go | — |
| CloakBrowser | Docker `cloakhq/cloakbrowser:latest` | C++ | — |
| Stealth JS | `~/src/go-browser/stealth/` | JS | — |
| Stealth profiles | `~/src/go-browser/stealth/profiles/` | JSON | — |
