# Go-Browser Stealth Specification

> Living document. Goal: make automated Chrome indistinguishable from a real human on a real Mac.

## Philosophy

**Not deception — authenticity.** Every property, every event, every timing pattern must match what a real Chrome 145 on a MacBook Pro (Intel, Catalina) would produce. If we can't perfectly replicate something, don't override it — let CloakBrowser's C++ patches handle it.

## Three-Layer Architecture

```
Layer 3: (removed — stealth_complement.js is the sole JS layer)
Layer 2: stealth_complement.js (6 modules, fills gaps CloakBrowser doesn't cover)
Layer 1: CloakBrowser C++ (33 chromium patches: canvas, webgl, audio, fonts, locale, CDP input)
```

**Rule:** Higher layers must not contradict lower layers. If CloakBrowser patches canvas at C++, don't also patch it in JS.

## Target Profile: Intel MacBook Pro, Chrome 145, Catalina

| Property | Value | Source |
|----------|-------|--------|
| Platform | `MacIntel` | CloakBrowser C++ + JS override |
| UA | `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36` | CloakBrowser C++ |
| sec-ch-ua | `"Chromium";v="145", "Google Chrome";v="145", "Not-A.Brand";v="24"` | JS override |
| sec-ch-ua-platform | `"macOS"` | JS override |
| Architecture | `x86` | JS override (userAgentData) |
| Screen | 1440×900 @2x (Retina 13") | JS override |
| availHeight | 875 (900 - 25px macOS menu bar) | JS override |
| colorDepth | 24 | JS override |
| GPU | Intel Iris OpenGL Engine | JS override (WebGL getParameter) |
| HW Concurrency | 8 | JS override |
| Device Memory | 8 GB | JS override |
| Languages | `["en-US", "en"]` | JS override |
| Timezone | America/Los_Angeles | CloakBrowser `--timezone` flag |
| Connection | wifi, 10 Mbps, 50ms RTT, 4g | JS override |
| Plugins | 5 (PDF viewers) | stealth_complement.js |
| maxTouchPoints | 0 (not touch device) | JS override |

### Profile JSON: `stealth/profiles/mac_chrome145.json`

## Stealth Modules

### `00_profile.js` — Profile Loader
- Reads `window.__stealthProfile` (set by Go via `EvalOnNewDocument`)
- Aliases to `window.__sp` for other modules
- Cleaned up after all modules load (in `05_worker_injection.js`)

### `01_cdp_markers.js` — CDP Marker Cleanup
- Removes `$cdc_`, `$chrome_`, `__webdriver`, `__selenium`, `__playwright`, `__pw_`
- MutationObserver watches for dynamically injected CDP attributes
- `Error.prepareStackTrace` setter blocked (prevents stack-based CDP detection)

### `02_navigator.js` — Navigator Property Overrides
**toString masking:** All getters wrapped via `__defineNativeGetter()` which stores native toString in a WeakMap. `Function.prototype.toString` patched to return `"function get X() { [native code] }"` for all overridden getters.

Overridden properties:
- `navigator.webdriver` → `false` (boolean, not undefined)
- `navigator.userAgentData` → full NavigatorUAData with `getHighEntropyValues()`
- `navigator.hardwareConcurrency` → from profile
- `navigator.deviceMemory` → from profile
- `navigator.maxTouchPoints` → from profile
- `navigator.languages` / `navigator.language` → from profile
- `navigator.platform` → from profile (`MacIntel`)
- `navigator.connection` → NetworkInformation proxy with type/downlink/rtt/effectiveType
- `navigator.mediaDevices` → stub with `enumerateDevices()` (3 devices)
- `screen.*` (width, height, availWidth, availHeight, colorDepth, pixelDepth) → from profile
- `window.devicePixelRatio` → from profile
- `window.outerWidth` / `window.outerHeight` → screen.width / screen.height+77
- `window.screenX` / `window.screenY` → 0, 25 (below macOS menu bar)
- `document.hasFocus()` → always `true` (headless returns false)

### `03_chrome_object.js` — Chrome Object Stubs
- `window.chrome.runtime` — connect(), sendMessage(), onMessage
- `window.chrome.csi()` — page timing (startE, onloadT, pageT)
- `window.chrome.loadTimes()` — navigation timing
- `window.chrome.app` — InstallState, RunningState, getDetails()

**Known issue:** csi() and loadTimes() return identical timestamps. Should have realistic offsets.

### `04_media_permissions.js` — Media & Permissions
- `HTMLMediaElement.canPlayType()` → 'probably' for h264/vp8/vp9
- `Notification.permission` → 'default' (headless returns 'denied')
- `Permissions.query({name: 'notifications'})` → matches Notification.permission

### `05_worker_injection.js` — Worker Thread Stealth
- Wraps `window.Worker` constructor
- Handles string URLs (fetch + prepend bootstrap), blob: URLs (importScripts), data: URLs
- Bootstrap patches in Worker context: `webdriver`, `hardwareConcurrency`, `deviceMemory`, `platform`, `languages`, `language`
- `Worker.toString()` → `"function Worker() { [native code] }"`
- `Worker.prototype` → `OriginalWorker.prototype` (instanceof check passes)
- Cleans up `__stealthProfile`, `__sp`, `__defineNativeGetter` at the end

## Behavioral Simulation

### Mouse Movement (`humanize/mouse.go`)
- Bezier curve paths (15-25 steps)
- Per-step delay: 8-16ms (matches 60Hz display refresh)
- Random target offset within element bounds (±30%)
- Idle drift: micro-movements every 2-5 seconds

### Typing (`actions_humanize.go`)
- CDP `InputDispatchKeyEvent` with full `keyDown` → `char` → `keyUp` sequence
- Per-character delay: 50-120ms base + 15% chance of 200-500ms pause
- Character → DOM Code mapping (`charToCode()`: KeyA-KeyZ, Digit0-9, Space, etc.)
- `WindowsVirtualKeyCode` set to uppercase ASCII

### Click (`actions_humanize.go`)
- Bezier path to element center (with jitter)
- CDP `mousePressed` + `mouseReleased` (isTrusted: true)
- Random click position offset within element

### Warmup (`actions_humanize.go`)
- Configurable duration (default 3000ms)
- Random mouse movements, occasional scrolls (20%), occasional clicks (10%)
- All CDP Input.dispatch events (isTrusted: true)

## CloakBrowser Configuration

**Mode: HEADED** (not headless) via Xvfb virtual framebuffer. This eliminates 8+ headless detection vectors natively without any JS overrides.

```yaml
# compose/search.yml
command: >
  bash -c "dbus-daemon --system --fork;
  dbus-daemon --session --address=unix:path=/tmp/dbus-session --fork;
  Xvfb :99 -screen 0 1440x900x24 -nolisten tcp &
  sleep 1 &&
  chrome
  --use-gl=angle --use-angle=swiftshader --enable-webgl
  --window-size=1440,900
  --fingerprint=79849
  --fingerprint-platform=macos
  --timezone=America/Los_Angeles
  --remote-allow-origins=*
environment:
  - TZ=America/Los_Angeles
```

## CDP Safety

| CDP Call | Detection Risk | Status |
|----------|---------------|--------|
| `Runtime.enable` | HIGH — Castle.io detects | NOT used by default. Optional via `SubscribeConsole()` |
| `Network.enable` | LOW | Used for network logging |
| `Input.dispatch*` | NONE — produces isTrusted:true | Used for all interactions |
| `Page.navigate` | NONE | Standard |
| `DOM.getDocument` | LOW | Used for snapshots |
| `Fetch.enable` | MEDIUM — for proxy auth | Used only when proxy has credentials |

## Detection Vectors Checklist

### Passing ✅
- [x] `navigator.webdriver` = false (getter toString = native code)
- [x] `eval.toString().length` = 33
- [x] `chrome.runtime`, `chrome.csi`, `chrome.loadTimes`, `chrome.app`
- [x] WebGL vendor/renderer spoofed
- [x] `navigator.plugins.length` = 5
- [x] `navigator.languages` = ["en-US", "en"]
- [x] `navigator.connection` present with wifi/10Mbps
- [x] `Notification.permission` = 'default'
- [x] CDP markers cleaned ($cdc_, $chrome_, etc.)
- [x] `Error.prepareStackTrace` blocked
- [x] `document.hasFocus()` = true
- [x] `outerWidth/outerHeight` realistic
- [x] Worker constructor toString = native code
- [x] Worker webdriver = false (blob/data URLs handled)
- [x] Stealth markers cleaned up (__sp, __stealthProfile)
- [x] Screen resolution consistent with macOS (1440x900@2x)
- [x] Profile internally consistent (x86 + Intel GPU + Catalina)
- [x] Keyboard events: full keyDown/char/keyUp sequence
- [x] Mouse: Bezier paths, human-like timing

### Known Gaps ⚠️
- [ ] `chrome.csi()` / `chrome.loadTimes()` return identical timestamps
- [ ] `navigator.userAgentData` is not `instanceof NavigatorUAData`
- [ ] `navigator.connection` is not `instanceof NetworkInformation`
- [ ] WebGL `getParameter` override detectable via toString (partial — toString masked for most but not WebGL)
- [ ] `getSupportedExtensions()` returns SwiftShader extensions, not Intel Iris
- [ ] `performance.memory` not spoofed (Chrome-specific)
- [ ] `speechSynthesis.getVoices()` returns empty (headless has no voices)
- [ ] `Accept-Language` HTTP header may not match `navigator.languages`
- [ ] `x-client-transaction-id` header not implemented

### Delegated to CloakBrowser C++
- Canvas fingerprint (toDataURL/toBlob)
- AudioContext fingerprint
- Font enumeration
- WebRTC local IP
- CDP Input events (isTrusted: true)
- Performance.now() precision

## Testing

### Fingerprint diagnostic
```bash
curl -s -X POST http://127.0.0.1:8906/chrome/interact \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com","profile":"mac_chrome145","actions":[
    {"type":"evaluate","js":"function(){...fingerprint checks...}"}
  ]}'
```

### Anti-detect test sites
- https://bot.sannysoft.com/
- https://abrahamjuliot.github.io/creepjs/
- https://browserleaks.com/

### Unit tests
```bash
cd ~/src/go-browser && go test ./... -count=1 -short  # 111 tests
```
