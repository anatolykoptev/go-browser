# go-browser Roadmap

## Completed Phases

### Phase 0: Bootstrap (v0.1.0) — DONE
- [x] `Browser`, `Page` interfaces
- [x] `remote/` backend (chromedp CDP)
- [x] Page pool with semaphore
- [x] Options: concurrency, timeout, user-agent

### Phase 1: Rod Backend (v0.2.0) — DONE
- [x] `rod/` backend — in-process Chromium via Rod
- [x] Proxy integration via go-stealth ProxyPool
- [x] Crash recovery with auto-restart
- [x] Request interception (block images/fonts)
- [x] Removed `browserless` container (384MB freed)

---

## Active: Browser Hybrid Architecture

**Spec:** `~/docs/superpowers/specs/2026-03-27-browser-hybrid-architecture-design.md`

### Phase 2: go-browser HTTP Service (v0.3.0)

**Plan:** `~/docs/superpowers/plans/2026-03-27-browser-hybrid-phase1.md`
**Goal:** Standalone service on `:8906` connected to CloakBrowser via rod CDP.

- [ ] HTTP server skeleton + health endpoint
- [ ] Session pool with TTL reaper
- [ ] Chrome manager (rod → CloakBrowser WS, stealth JS, per-context proxy)
- [ ] 15 Chrome actions matching ox-browser API contract
- [ ] Humanize engine (Bezier mouse, keyboard timing)
- [ ] /chrome/interact handler
- [ ] /solve handler (CF clearance)
- [ ] /render handler
- [ ] Dockerfile + Docker Compose
- [ ] Integration smoke tests

**Parallelizable:** Session pool, chrome manager, actions, humanize — all after skeleton.
**Resource:** +256MB (go-browser container)

### Phase 3: ox-browser Decoupling (v0.4.0)

**Goal:** ox-browser proxies Chrome ops to go-browser, removes chromiumoxide.

- [ ] ox-browser /solve → proxy to go-browser:8906/solve
- [ ] ox-browser /chrome/interact → proxy to go-browser
- [ ] Remove chromiumoxide from ox-browser Cargo.toml
- [ ] Delete ~2800 lines: browser_pool, chrome_session, solver_chromium, chrome_interact
- [ ] Move twitter login to go-browser (or separate service)
- [ ] Rebuild ox-browser — verify faster builds, all HTTP endpoints work
- [ ] Update go-stealth: OxBrowserSolver backed by go-browser

**Result:** ox-browser = pure HTTP stealth (wreq). No abandoned deps. Faster builds.

### Phase 4: Cleanup (v0.5.0)

**Goal:** Remove Byparr, optimize, add MCP.

- [ ] Remove Byparr container — free 1.5GB RAM
- [ ] go-browser MCP server (chrome_interact, solve, render as MCP tools)
- [ ] go-wp/go-search import go-browser directly (drop browser_init.go boilerplate)
- [ ] Prometheus metrics (sessions, latency, errors)
- [ ] Connection pooling, page pre-warming
- [ ] Proxy auth via rod Hijack (Fetch.authRequired)

## Resource Impact

| Container | Now | Phase 2 | Phase 4 |
|-----------|-----|---------|---------|
| ox-browser | 768MB | 768MB | 512MB |
| cloakbrowser | 512MB | 512MB | 512MB |
| byparr | 1536MB | 1536MB | **0** |
| go-browser | 0 | 256MB | 256MB |
| **Total** | **2816MB** | **3072MB** | **1280MB** |

## Future

- **Lightpanda backend** — when stable, 9x less memory for simple renders
- **Kalamari** — pure Rust headless, 10MB binary, watch for maturity
- **Multi-browser** — CloakBrowser (Chromium) + Camoufox (Firefox) when Camoufox reaches production-ready
