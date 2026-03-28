# Stealth Roadmap — Go-Browser

> From "looks like a bot" to "indistinguishable from a real person on a MacBook"

## Current State

**STEALTH-SPEC.md**: 20 detection vectors passing, 9 known gaps. Castle.io SDK generates tokens consistently but Twitter returns 399. All open-source login solutions (twikit, d60/twitter_login) also broken — Castle protocol updated.

## Phase 1: Quick JS Fixes (1 day, HIGH impact)

Low-effort fixes that close obvious detection vectors. All in stealth JS modules.

### 1.1 `performance.memory` spoof
- **File**: `stealth/02_navigator.js`
- **What**: Add realistic heap size values (jsHeapSizeLimit ~4GB, totalJSHeapSize ~20MB)
- **Detectors**: Akamai, FingerprintJS Pro
- **Effort**: 5 lines

### 1.2 Worker `userAgent` patch
- **File**: `stealth/05_worker_injection.js`
- **What**: Add `navigator.userAgent` override to worker bootstrap (currently missing — only webdriver, hwConcurrency, deviceMemory, platform, languages are patched)
- **Detectors**: Castle.io, DataDome, PerimeterX — all check Worker UA vs main thread
- **Effort**: 2 lines

### 1.3 `document.visibilityState` + `document.hidden`
- **File**: `stealth/02_navigator.js`
- **What**: Force `visibilityState='visible'`, `hidden=false`, suppress `visibilitychange` events
- **Detectors**: Kasada, PerimeterX
- **Effort**: 5 lines

### 1.4 `chrome.csi()` / `chrome.loadTimes()` realistic timings
- **File**: `stealth/03_chrome_object.js`
- **What**: Use `performance.timing` values instead of `Date.now()` for all timing fields. Add realistic deltas between navigation stages.
- **Detectors**: Castle.io (part of fingerprint)
- **Effort**: 15 lines

### 1.5 `speechSynthesis.getVoices()` stub
- **File**: `stealth/04_media_permissions.js`
- **What**: Return 10-15 macOS Apple voices (Alex, Samantha, Victoria, etc.) instead of empty array
- **Detectors**: CreepJS, PerimeterX
- **Effort**: 20 lines

### 1.6 `Accept-Language` header via CDP
- **File**: `chrome.go` (NewStealthPage)
- **What**: Call `Network.setExtraHTTPHeaders` with `Accept-Language: en-US,en;q=0.9` matching profile languages
- **Detectors**: All — trivial cross-check
- **Effort**: 5 lines Go

### 1.7 `navigator.getBattery()` stub
- **File**: `stealth/02_navigator.js`
- **What**: Return Promise with `{charging: true, level: 0.9, chargingTime: 0, dischargingTime: Infinity}`
- **Detectors**: Rare but cheap to fix
- **Effort**: 5 lines

### 1.8 `navigator.getGamepads()` stub
- **File**: `stealth/02_navigator.js`
- **What**: Return `[null, null, null, null]` (4 empty slots, matches real Chrome)
- **Effort**: 1 line

### 1.9 Localhost port scan protection
- **File**: `stealth/01_cdp_markers.js`
- **What**: Intercept fetch/XMLHttpRequest/WebSocket to localhost/127.0.0.1/[::1], return network error
- **Detectors**: PerimeterX, eBay
- **Effort**: 10 lines

## Phase 2: Medium Fixes (2-3 days)

### 2.1 WebGL `getSupportedExtensions()` — Intel Iris list
- **File**: `stealth/02_navigator.js`
- **What**: Override `getSupportedExtensions()` to return the exact extension list for Intel Iris 540 (MacBook Pro 2016). SwiftShader returns a different set. Also override other `getParameter()` constants (MAX_TEXTURE_SIZE, MAX_RENDERBUFFER_SIZE, etc.)
- **Detectors**: CreepJS, Browserleaks
- **Effort**: 30 lines (need reference data from a real Intel Mac)

### 2.2 `navigator.userAgentData` instanceof fix
- **File**: `stealth/02_navigator.js`
- **What**: Capture `NavigatorUAData.prototype` before overriding, create spoofed object with correct prototype chain so `instanceof NavigatorUAData` returns true
- **Detectors**: CreepJS (prototype lie detection)
- **Effort**: 15 lines, tricky

### 2.3 `navigator.connection` instanceof fix
- **File**: `stealth/02_navigator.js`
- **What**: Same approach — capture `NetworkInformation.prototype`, create object with correct chain
- **Effort**: 10 lines

### 2.4 CSS media queries via CDP Emulation
- **File**: `chrome.go` (NewStealthPage)
- **What**: Call `Emulation.setEmulatedMedia` to ensure `(hover: hover)`, `(pointer: fine)`, `(display-mode: browser)` match a real laptop. Also set `prefers-color-scheme: light`.
- **Detectors**: PerimeterX
- **Effort**: 10 lines Go

### 2.5 Sec-CH-UA GREASE randomization
- **File**: `stealth/profiles/mac_chrome145.json` + `02_navigator.js`
- **What**: Randomize the GREASE brand format per session from the set of valid Chrome 145 patterns. Not-A.Brand / Not A(Brand) / Not)A;Brand etc.
- **Detectors**: DataDome
- **Effort**: 10 lines

### 2.6 WebGL getParameter toString masking
- **File**: `stealth/02_navigator.js`
- **What**: Apply `__defineNativeGetter` pattern to WebGL `getParameter` override so `.toString()` returns `[native code]`
- **Effort**: 5 lines

## Phase 3: Infrastructure (1 week)

### 3.1 Verify HTTP/2 SETTINGS frame fingerprint
- **Action**: Test CloakBrowser against `https://tls.peet.ws/api/all` and `https://www.browserscan.net/tls`. Compare SETTINGS frame values (HEADER_TABLE_SIZE, INITIAL_WINDOW_SIZE, MAX_HEADER_LIST_SIZE) with real Chrome 145.
- **If mismatch**: Requires CloakBrowser C++ patch or Chrome flags
- **Detectors**: Akamai Bot Manager

### 3.2 Verify QUIC/HTTP3 behavior through proxies
- **Action**: Test if CloakBrowser attempts QUIC through Webshare proxy. Real Chrome uses QUIC for Google/Cloudflare domains. SOCKS5 breaks UDP → Chrome falls back to HTTP/2 silently. This pattern is detectable.
- **If issue**: Use HTTPS proxy instead of SOCKS5, or add `--disable-quic` flag (less suspicious than always-HTTP/2-never-QUIC)

### 3.3 Audit go-rod Runtime.enable calls
- **Action**: Grep all go-rod methods that internally call `Runtime.enable` (`page.Console()`, `page.Log()`, etc.). Ensure none are used in production flow.
- **Note**: Chrome 145+ V8 patch killed the main `Runtime.enable` detection vector, but alternative detection methods exist.

### 3.4 Webshare IP quality check
- **Action**: Test 10 random Webshare ports against `https://ipqualityscore.com/api` and `https://ipapi.com`. Check if IPs are flagged as proxy/VPN/datacenter.
- **If flagged**: Consider ISP-grade proxy provider (e.g., Bright Data ISP proxies)

### 3.5 OffscreenCanvas in Worker consistency
- **Action**: Test if CloakBrowser's canvas noise applies to OffscreenCanvas inside Workers. If not, canvas fingerprint from Worker differs from main thread.
- **Detectors**: CreepJS

## Phase 4: Anti-Bot Detector Tool (1 week)

New MCP tool for go-code: given a URL, identify what anti-bot protection is deployed.

### 4.1 Signature database
- **Source 1**: `scrapfly/Antibot-Detector` JSON files (16 anti-bot services, MIT license)
- **Source 2**: `projectdiscovery/wappalyzergo` (6000+ technologies, Go library)
- **Source 3**: Custom Castle.io signatures (not in any existing tool)
- **Format**: JSON with `cookie[]`, `header[]`, `url[]`, `content[]`, `script_src[]` + confidence scores

### 4.2 Static analyzer (Phase 1 — HTTP only)
- **Location**: `go-code/internal/antibot/`
- **Input**: URL → HTTP GET via go-stealth
- **Analysis**: Response headers + Set-Cookie + HTML body + `<script src>` URLs
- **Output**: `[{name, confidence, category, signals}]`
- **Coverage**: ~70% of anti-bot services (all header/cookie-based)
- **Effort**: ~600 lines Go

### 4.3 Dynamic analyzer (Phase 2 — browser)
- **Location**: `go-browser` action or `ox-browser` endpoint
- **Input**: URL → full page load via CloakBrowser
- **Analysis**: CDP Network.requestWillBeSent (SDK endpoint URLs) + Runtime.evaluate (JS global vars like `window._pxAppId`, `window.dd`) + final cookies
- **Output**: Same format + network-based signals
- **Coverage**: ~95% of anti-bot services
- **Effort**: ~300 lines Go + CDP integration

### Detectable services (target list)
| Service | Static (HTTP) | Dynamic (Browser) |
|---------|:---:|:---:|
| Cloudflare | ✅ | ✅ |
| DataDome | ✅ (headers) | ✅ (JS + cookies) |
| Akamai | ✅ (cookies) | ✅ (JS + network) |
| PerimeterX/HUMAN | ✅ (cookies) | ✅ (JS + network) |
| Castle.io | ❌ | ✅ (script src + network) |
| Kasada | ❌ | ✅ (network endpoints) |
| Shape/F5 | ✅ (headers) | ✅ (JS + cookies) |
| FingerprintJS Pro | ❌ | ✅ (script src + JS) |
| Imperva/Incapsula | ✅ (cookies) | ✅ |
| AWS WAF | ✅ (cookies) | ✅ |
| reCAPTCHA | ✅ (script src) | ✅ |
| hCaptcha | ✅ (script src) | ✅ |
| Turnstile | ✅ (script src) | ✅ |

## Phase 5: Continuous Validation

### 5.1 Anti-detect test suite
Automated tests against:
- `https://bot.sannysoft.com/` — basic headless detection
- `https://abrahamjuliot.github.io/creepjs/` — advanced fingerprint analysis
- `https://browserleaks.com/` — WebGL, canvas, fonts, screen
- `https://tls.peet.ws/api/all` — TLS/HTTP2 fingerprint

### 5.2 Regression testing
After each stealth change:
1. Run fingerprint diagnostic (in-browser JS eval)
2. Check creepJS trust score (should be >50%)
3. Verify Castle.io token generation (no hanging)
4. Test Twitter login flow (track 399 vs success)

### 5.3 Monitoring Castle.io / Twitter updates
- Watch `d60/twitter_login` repo for Castle protocol updates
- Monitor `yubie-re/castleio-gen` (archived but may get forked)
- Check Castle.io changelog for SDK version changes
