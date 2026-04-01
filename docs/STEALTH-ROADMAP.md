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

## Phase 6: Adaptive Anti-Detection Engine — PLANNED (2026-04)

Triggered by LinkedIn login failure: PerimeterX blocks CDP automation despite
headed Chrome + stealth injections. Root causes identified via security_scan:

### 6.1 Runtime.enable Isolation (CRITICAL)

**Problem:** Rod uses `Runtime.enable` globally which PX detects via
`Error.stack` getter side-effect. Phase 3.3 claimed done but rod re-enables
it on page navigation.

**Fix:** Replace rod's default `Runtime.enable` with isolated execution:
- Use `Page.createIsolatedWorld` for JS evaluation
- Avoid `Runtime.enable` / `Runtime.callFunctionOn` on main world
- Reference: `rebrowser/rebrowser-patches` (Node.js implementation)

**Impact:** Fixes `page.Eval()`, `el.Click()`, `el.Input()` hanging on PX pages.

### 6.2 Adaptive Input Strategy (HIGH)

**Problem:** Some sites block CDP `Input.dispatchKeyEvent`, others block
JS `execCommand('insertText')`. No single input method works everywhere.

**Fix:** Auto-detect required strategy from security_scan results:
```go
type InputStrategy int
const (
    InputRod     InputStrategy = iota // Default: rod el.Input() — fast
    InputCDP                          // CDP dispatchKeyEvent — PX-safe
    InputInsert                       // CDP Input.insertText — React-safe
)

// DetectInputStrategy runs before interaction to choose strategy.
func DetectInputStrategy(scanResult *SecurityScanResult) InputStrategy {
    if scanResult.HasDetection("PerimeterX") || scanResult.HasDetection("HUMAN") {
        return InputCDP
    }
    if scanResult.SpoofingDetected {
        return InputCDP
    }
    return InputRod
}
```

Integrate with `chrome_interact`: on first action per session, run quick
security check (HTTP-only, <1s) and cache strategy for the domain.

### 6.3 Screen/DPR Consistency (MEDIUM)

**Problem:** `screen.width/height` vs CSS `@media` mismatch detected by PX.
`devicePixelRatio` doesn't match CSS resolution query.

**Fix:** Align `Emulation.setDeviceMetricsOverride` with actual screen:
- Set `width/height` matching `window.innerWidth/Height`
- Set `deviceScaleFactor` matching real monitor (1.0 for headed, 2.0 for retina)
- On Chrome 142+: use `--screen-info` launch flag
- Verify: `window.screen.width * devicePixelRatio` should equal physical resolution

### 6.4 Canvas/WebGL Lie Detection Fix (MEDIUM)

**Problem:** PX detects `canvas.toDataURL.toString()` as tampered.
CloakBrowser overrides return non-native `toString()`.

**Fix options:**
1. **Don't randomize canvas on non-fingerprint sites** — LinkedIn doesn't
   need unique canvas, just consistent one. Disable canvas override for
   domains without FingerprintJS Pro detection.
2. **Proxy approach:** Use `Proxy()` wrapper that preserves native toString:
   ```js
   const proxy = new Proxy(original, { apply(target, thisArg, args) { ... } });
   Object.defineProperty(proto, 'toDataURL', { value: proxy });
   ```
3. **Chromium-level patch** (CloakBrowser) — most reliable, no JS detection.

### 6.5 PX Challenge Cookie Flow (MEDIUM)

**Problem:** LinkedIn login requires `_px3` clearance cookie before form
submit is accepted. Without it, server redirects back to `/login`.

**Fix:** After page load, poll for `_px3` cookie via CDP `Network.getCookies`:
```go
func WaitForPXClearance(ctx context.Context, page *rod.Page, timeout time.Duration) bool {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        cookies, _ := proto.NetworkGetCookies{}.Call(page)
        for _, c := range cookies.Cookies {
            if c.Name == "_px3" { return true }
        }
        time.Sleep(500 * time.Millisecond)
    }
    return false
}
```

### 6.6 Adaptive Security Pre-Flight (LOW)

**Problem:** Currently `type_text` uses `slowly=true` flag manually.
User must know which sites need CDP input.

**Fix:** New `adapt` action type that auto-configures session:
```json
{"type": "adapt", "url": "https://www.linkedin.com/login"}
```
This runs a quick HTTP security scan (<1s), caches results per domain,
and configures session's input strategy, stealth profile, and wait behavior
automatically. Subsequent actions in the session use the detected strategy.

### Implementation Order

1. **6.1** Runtime.enable isolation — unlocks everything else
2. **6.5** PX cookie polling — quick win for LinkedIn
3. **6.2** Adaptive input — auto-select strategy
4. **6.3** Screen consistency — stealth profile fix
5. **6.4** Canvas lie — CloakBrowser patch preferred
6. **6.6** Adapt action — full auto-detection integration

### Validated Findings (2026-04-01 LinkedIn session)

- Chrome 145 on server — Input.coordinatesLeak bug already fixed
- `type_text slowly=true` with CDP dispatchKeyEvent: **works** (no hang)
- `type_text slowly=false` with rod el.Input(): **hangs** on PX pages
- `page.Eval()` / `evaluate` action: **intermittently hangs** (PX timing)
- `el.Click()` with humanize: **hangs** (rod Runtime.callFunctionOn)
- JS `button.click()` via evaluate: works but PX rejects submit (no `_px3`)
- `execCommand('insertText')`: fills DOM but not React state
- Security scan detected: PerimeterX (PXdOjV695v), Canvas/Screen/DPR lies

---

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
