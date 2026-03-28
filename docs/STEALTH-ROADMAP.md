# Stealth Roadmap — Go-Browser

> From "looks like a bot" to "a real person on a real MacBook"

## Current State (2026-03-28)

- **Headed Chrome** via Xvfb + persistent profile (Docker volume)
- **bot.sannysoft.com**: 56/56 PASS
- **TLS/HTTP2**: identical to real Chrome 145 (JA4, Akamai hash verified)
- **Scraping**: Amazon, Booking, Avito, Reddit, HN, GitHub — 0 blocks
- **Twitter 399**: all open-source solutions broken, not our issue

## Phase 1: Quick JS Fixes — ✅ DONE

| # | Fix | Status |
|---|-----|--------|
| 1.1 | `performance.memory` | ✅ NATIVE (headed Chrome) |
| 1.2 | Worker `userAgent` | ✅ Done |
| 1.3 | `visibilityState` | ✅ NATIVE (headed Chrome) |
| 1.4 | `csi()/loadTimes()` realistic timings | ✅ Done |
| 1.5 | `speechSynthesis.getVoices()` | ✅ NATIVE (headed Chrome, 9 voices) |
| 1.6 | `Accept-Language` header | ✅ Done (CDP) |
| 1.7 | `getBattery()` | ✅ Done |
| 1.8 | `getGamepads()` | ✅ Done |
| 1.9 | Localhost port scan protection | ✅ Done |

## Phase 2: Medium Fixes — OPEN

| # | Fix | Status | Priority |
|---|-----|--------|----------|
| 2.1 | WebGL `getSupportedExtensions()` Intel Iris list | ❌ TODO | MEDIUM — CreepJS checks |
| 2.2 | `navigator.userAgentData` instanceof fix | ❌ TODO | MEDIUM — CreepJS prototype lie |
| 2.3 | `navigator.connection` instanceof fix | ❌ TODO | LOW — partial (value works, instanceof fails) |
| 2.4 | CSS media queries via CDP Emulation | ✅ NATIVE (headed Chrome) | — |
| 2.5 | Sec-CH-UA GREASE randomization (go-browser) | ❌ TODO | MEDIUM — DataDome |
| 2.6 | WebGL getParameter toString masking | ❌ TODO | LOW |

**Remaining: 5 items.** 2.1 and 2.2 are highest priority.

### 2.1 WebGL getSupportedExtensions() — Intel Iris 540

Our CloakBrowser uses SwiftShader (software renderer) but we spoof the GPU as "Intel Iris OpenGL Engine". SwiftShader and Intel Iris return **different WebGL extension lists**. CreepJS and Browserleaks check this.

**Fix**: Override `getSupportedExtensions()` to return exact Intel Iris 540 extension list. Also override `getParameter()` for MAX_TEXTURE_SIZE, MAX_RENDERBUFFER_SIZE etc.

**Need**: Reference data from a real Intel MacBook Pro. Run browserleaks.com/webgl on a real Mac and save the extension list + parameter values.

### 2.2 NavigatorUAData instanceof

`navigator.userAgentData instanceof NavigatorUAData` returns false (we return a plain object). CreepJS flags this as "prototype lie".

**Fix**: Before overriding, capture `Object.getPrototypeOf(navigator.userAgentData).constructor` and use it to create our spoofed object with the correct prototype chain. Tricky because CloakBrowser may not expose NavigatorUAData natively in headless.

### 2.5 GREASE Randomization (go-browser stealth JS)

Currently our `sec-ch-ua` in stealth profiles uses a static GREASE brand. Should randomize per-session from the set of valid Chrome 145 patterns.

Note: ox-browser already has GREASE randomization in `profile_hints.rs` (done this session). go-browser stealth JS needs the same.

## Phase 3: Infrastructure — PARTIAL

| # | Check | Status | Result |
|---|-------|--------|--------|
| 3.1 | HTTP/2 SETTINGS frame | ✅ Verified | Identical to real Chrome |
| 3.2 | QUIC/HTTP3 through proxies | ❌ Not tested | May differ from real Chrome |
| 3.3 | Runtime.enable audit | ✅ Done | Removed from default path |
| 3.4 | Webshare IP quality | ❌ Not tested | Some ports blocked by Twitter |
| 3.5 | OffscreenCanvas Worker | ❌ Not tested | CloakBrowser C++ may not cover |
| 3.6 | Persistent Chrome profile | ✅ Done | Docker volume `cloakbrowser_profile` |

## Phase 4: Anti-Bot Detector Tool — PLANNED

New MCP tool: given a URL, identify what anti-bot protection is deployed.

### Architecture
```
Phase 1 (HTTP only):     ox-browser /fetch → headers + cookies + HTML → signature matching
Phase 2 (Browser):       go-browser /chrome/interact → CDP network + JS vars → deep detection
```

### Data sources
1. `scrapfly/Antibot-Detector` — 16 anti-bot JSON signatures (MIT)
2. `projectdiscovery/wappalyzergo` — 6000+ technology signatures (Go library)
3. Custom Castle.io / Kasada signatures

### Target services (13)
Cloudflare, DataDome, Akamai, PerimeterX/HUMAN, Castle.io, Kasada, Shape/F5, FingerprintJS Pro, Imperva/Incapsula, AWS WAF, reCAPTCHA, hCaptcha, Turnstile

### Implementation
- **Location**: `go-code/internal/antibot/` (static) + `go-browser` action (dynamic)
- **Effort**: ~600 lines Go (static) + ~300 lines Go (dynamic)
- **Dependency**: `projectdiscovery/wappalyzergo` for base coverage

## Phase 5: Continuous Validation — PLANNED

### 5.1 Anti-detect test suite
Automated tests against:
- `bot.sannysoft.com` — basic headless detection (currently 56/56)
- `abrahamjuliot.github.io/creepjs` — advanced fingerprint analysis
- `browserleaks.com` — WebGL, canvas, fonts, screen
- `tls.peet.ws/api/all` — TLS/HTTP2 fingerprint

### 5.2 Regression testing
After each stealth change:
1. Run fingerprint diagnostic (in-browser JS eval)
2. Check creepJS trust score (should be >50%)
3. Verify Castle.io token generation (no hanging)
4. Scraping test: Amazon, Booking, Reddit (no blocks)

### 5.3 Monitoring
- `d60/twitter_login` repo — Castle protocol updates
- CloakBrowser releases — new C++ patches
- Chrome stable channel — version updates for profiles

## Completed This Session (2026-03-28)

### go-browser
- [x] Headed Chrome via Xvfb + dbus
- [x] Persistent Chrome profile (Docker volume)
- [x] Function.prototype.toString native masking (WeakMap)
- [x] Worker proxy: Blob/data: URLs + full navigator props + userAgent
- [x] Worker.toString() = native code
- [x] InputDispatchKeyEvent (keyDown/char/keyUp) replaces InputInsertText
- [x] Profile consistency: x86, 1440x900, colorDepth 24, platformVersion 10.15.7
- [x] Accept-Language via CDP setExtraHTTPHeaders
- [x] getBattery/getGamepads stubs
- [x] Localhost port scan protection (fetch + WebSocket)
- [x] chrome.csi/loadTimes realistic timing from performance.timing
- [x] navigator.platform override
- [x] Stealth markers cleanup (__sp, __stealthProfile, __defineNativeGetter)
- [x] Removed isTrusted:false castle events (06_castle_events.js deleted)
- [x] Removed RuntimeEnable from SubscribeCDP
- [x] navigator.connection spoofing
- [x] eval_on_new_document action type
- [x] TLS/HTTP2 fingerprint verified identical to real Chrome

### go-social
- [x] Castle token generator (Go port, 33 tests)
- [x] Pure API login (ui_metrics solver + flow state machine, 32 tests)
- [x] x-client-transaction-id generator
- [x] Replaced botwitter.com with local generator

### ox-browser
- [x] Chrome 145 profiles (was 131/133)
- [x] Firefox 138 profiles (was 133)
- [x] Accept-Language middleware
- [x] GREASE brand randomization
- [x] sec-ch-ua-full-version-list header
- [x] Removed deprecated Twitter login code (-1718 lines)

### Documentation
- [x] STEALTH-SPEC.md — full detection vector checklist
- [x] STEALTH-ROADMAP.md — this file
- [x] RESEARCH-LOG.md — 289 lines investigation history
- [x] ARCHITECTURE.md — two-tier browser architecture
