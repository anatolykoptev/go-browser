# Antibot Gap Solutions — Research & Payloads

Research context for Phase 2 implementation (go-browser/docs/plans/2026-04-09-antibot-phase2.md).
Source: research-agent a0e8fff81242ec738, 2026-04-09.

## Gap 1: WebRTC Local IP Leak

**Why**: Cloudflare Ent / DataDome / FingerprintJS Pro compare WebRTC-derived public IP to TCP source IP. Mismatch = proxy. Also detect `.local` mDNS (residential), empty lists (headless signal), `getStats()` inconsistencies.

**Solution — layered**:

1. **Flag layer** (preferred baseline):
   ```
   --force-webrtc-ip-handling-policy=disable_non_proxied_udp
   --webrtc-ip-handling-policy=disable_non_proxied_udp
   ```
   But empty candidate list is itself rare (~3% real users) → DataDome flags.

2. **CloakBrowser native** (recommended): pass `--fingerprint-webrtc-ip=<PROXY_EXIT_IP>`. C++ injects synthetic `srflx` candidate matching Webshare exit IP, `getStats()` reports matching `remoteCandidateId`. Use Webshare port→IP resolver.

3. **JS fallback wrapper** (belt-and-suspenders, covers also legacy `webkitRTCPeerConnection`, workers, iframes):

```js
(() => {
  const wrap = (OrigPC) => {
    if (!OrigPC) return OrigPC;
    const Wrapped = function(config, ...rest) {
      if (config && Array.isArray(config.iceServers)) {
        config.iceServers = config.iceServers.filter(s => {
          const urls = [].concat(s.urls || s.url || []);
          return urls.every(u => !/^stun:/i.test(u));
        });
      }
      const pc = new OrigPC(config, ...rest);
      const origAdd = pc.addEventListener.bind(pc);
      pc.addEventListener = (type, cb, ...r) => {
        if (type === 'icecandidate') {
          return origAdd(type, (ev) => {
            if (ev.candidate && ev.candidate.candidate) {
              const c = ev.candidate.candidate;
              if (/\.local\b/.test(c)) return;
              if (/\b(10|127|192\.168|172\.(1[6-9]|2\d|3[01]))\./.test(c)) return;
            }
            cb(ev);
          }, ...r);
        }
        return origAdd(type, cb, ...r);
      };
      Object.defineProperty(pc, 'onicecandidate', {
        set(fn) { pc.addEventListener('icecandidate', fn); },
        configurable: true,
      });
      return pc;
    };
    Wrapped.prototype = OrigPC.prototype;
    Object.setPrototypeOf(Wrapped, OrigPC);
    Wrapped.toString = () => OrigPC.toString();
    return Wrapped;
  };
  window.RTCPeerConnection = wrap(window.RTCPeerConnection);
  window.webkitRTCPeerConnection = wrap(window.webkitRTCPeerConnection);
})();
```

**Gotchas**: `icegatheringstate` must progress `new → gathering → complete` with realistic timing (50-500ms) — instant complete = bot. Wrap BOTH RTCPeerConnection and webkitRTCPeerConnection. getStats() must stay consistent.

**Verification**: `browserleaks.com/webrtc` — only proxy exit IP, no local. CreepJS WebRTC panel.

---

## Gap 2: Timezone + Locale CDP Enforcement

**Why**: CreepJS top-3 "lies" panel. Compares `Intl.DateTimeFormat().resolvedOptions().timeZone` vs IP-geoIP TZ. Also `Date.prototype.toString()` offset, `getTimezoneOffset()`, `navigator.language(s)`, `Accept-Language` header — ALL must agree.

**CDP calls** (per page AND per auto-attached child target):
```json
{"method":"Emulation.setTimezoneOverride",
 "params":{"timezoneId":"Europe/Moscow"}}

{"method":"Emulation.setLocaleOverride",
 "params":{"locale":"ru-RU"}}

{"method":"Emulation.setGeolocationOverride",
 "params":{"latitude":55.7558,"longitude":37.6173,"accuracy":50}}

{"method":"Emulation.setUserAgentOverride",
 "params":{
   "userAgent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
   "acceptLanguage":"ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7",
   "platform":"MacIntel",
   "userAgentMetadata": { /* see Gap 5b */ }
 }}
```

**Critical call ordering**:
1. `Target.setAutoAttach {autoAttach:true, waitForDebuggerOnStart:true, flatten:true}`
2. On `Target.attachedToTarget`: all 4 Emulation commands on new session
3. `Runtime.runIfWaitingForDebugger`
4. `Page.addScriptToEvaluateOnNewDocument` (languages shim)
5. `Page.navigate`

**Gotchas**:
- `setLocaleOverride` does NOT set `navigator.language` — need JS shim
- Setting TZ after load leaves stale Date objects → always before navigate
- SW doesn't propagate `setUserAgentOverride` ([puppeteer#8867](https://github.com/puppeteer/puppeteer/issues/8867))

**JS shim for `navigator.languages`**:
```js
(() => {
  const langs = Object.freeze(['ru-RU','ru','en-US','en']);
  Object.defineProperty(Navigator.prototype, 'language',
    { get: () => 'ru-RU', configurable: true });
  Object.defineProperty(Navigator.prototype, 'languages',
    { get: () => langs, configurable: true });
})();
```

---

## Gap 3: Fonts List Spoofing

**Why**: CreepJS `fonts/index.ts` probes 200+ OS-specific fonts via `document.fonts.check('0px "FontName"')` AND `FontFace(name, 'local("name")').load()`. macOS 13 (Ventura) signature fonts: `Apple SD Gothic Neo ExtraBold`, `STIX Two Math Regular`, `Noto Sans Canadian Aboriginal Regular`. Linux missing these → `PlatformClassifier: Linux` while UA=macOS → instant lie.

**Canonical probe list**: https://raw.githubusercontent.com/abrahamjuliot/creepjs/master/src/fonts/index.ts

**Recommended strategy: hybrid (c)+(a)**:
1. Install real macOS-equivalent fonts in cloakbrowser Dockerfile:
   - Apple open fonts (SF Pro, SF Mono, New York) from https://developer.apple.com/fonts/ — legally redistributable
   - `COPY fonts/ /usr/share/fonts/macos/ && fc-cache -f`
2. Remove Linux-only fonts: `apt-get remove fonts-liberation fonts-ubuntu fonts-dejavu*`
3. JS shim to hide residual Linux fonts:

```js
(() => {
  const HIDDEN = new Set([
    'Arimo','Chilanka','Cousine','Jomolhari','Liberation Mono',
    'Ubuntu','DejaVu Sans','Noto Color Emoji','MONO',
  ]);
  const origCheck = FontFaceSet.prototype.check;
  FontFaceSet.prototype.check = function(font, text) {
    for (const f of HIDDEN) if (font.includes(`"${f}"`)) return false;
    return origCheck.call(this, font, text);
  };
  const origForEach = FontFaceSet.prototype.forEach;
  FontFaceSet.prototype.forEach = function(cb, thisArg) {
    return origForEach.call(this, function(ff, k, set) {
      if (HIDDEN.has(ff.family)) return;
      return cb.call(thisArg, ff, k, set);
    });
  };
})();
```

**Gotcha**: Apple Color Emoji is NOT legally redistributable → fallback hide the font entirely (accept lowered entropy).

---

## Gap 4: AudioContext

**CloakBrowser C++ engine already handles this** via `--fingerprint=79849` seeded noise in WebAudio backend. Source: DeepWiki cloakhq/cloakbrowser/5-stealth-system.

**Action**: SKIP JS layer. Verification only:
```js
const ctx = new OfflineAudioContext(1, 44100, 44100);
const osc = ctx.createOscillator();
const comp = ctx.createDynamicsCompressor();
osc.connect(comp); comp.connect(ctx.destination);
osc.start(0);
const buf = await ctx.startRendering();
const sum = buf.getChannelData(0).slice(4500,5000).reduce((a,b)=>a+Math.abs(b),0);
// Same profile → same sum. Different profile → different.
```

---

## Gap 5a: navigator.plugins / mimeTypes

Chrome 145 ships **5 hardcoded PDF plugin entries** (since Chrome 92+).

```js
(() => {
  const fakeData = [
    { name:'PDF Viewer',          filename:'internal-pdf-viewer', description:'Portable Document Format' },
    { name:'Chrome PDF Viewer',   filename:'internal-pdf-viewer', description:'Portable Document Format' },
    { name:'Chromium PDF Viewer', filename:'internal-pdf-viewer', description:'Portable Document Format' },
    { name:'Microsoft Edge PDF Viewer', filename:'internal-pdf-viewer', description:'Portable Document Format' },
    { name:'WebKit built-in PDF', filename:'internal-pdf-viewer', description:'Portable Document Format' },
  ];
  const mimeTypes = [
    { type:'application/pdf', suffixes:'pdf', description:'Portable Document Format' },
    { type:'text/pdf',        suffixes:'pdf', description:'Portable Document Format' },
  ];
  const makePlugin = (d) => {
    const p = Object.create(Plugin.prototype);
    Object.defineProperties(p, {
      name:        { value: d.name,        enumerable: true },
      filename:    { value: d.filename,    enumerable: true },
      description: { value: d.description, enumerable: true },
      length:      { value: 2 },
    });
    return p;
  };
  const plugins = fakeData.map(makePlugin);
  const pluginArray = Object.create(PluginArray.prototype);
  plugins.forEach((p, i) => pluginArray[i] = p);
  Object.defineProperty(pluginArray, 'length', { value: plugins.length });
  pluginArray.item      = (i) => plugins[i] || null;
  pluginArray.namedItem = (n) => plugins.find(p => p.name === n) || null;
  pluginArray.refresh   = () => {};
  Object.defineProperty(Navigator.prototype, 'plugins',
    { get: () => pluginArray, configurable: true });
  const mtArray = Object.create(MimeTypeArray.prototype);
  mimeTypes.forEach((m, i) => mtArray[i] = m);
  Object.defineProperty(mtArray, 'length', { value: mimeTypes.length });
  Object.defineProperty(Navigator.prototype, 'mimeTypes',
    { get: () => mtArray, configurable: true });
})();
```

---

## Gap 5b: sec-ch-ua Client Hints — exact Chrome 145 macOS struct

**Critical**: cloakbrowser `--fingerprint-platform=macos` fixes JS-side navigator.platform + WebGL, but does NOT fix HTTP `sec-ch-ua-platform` headers. Must set `userAgentMetadata` via CDP.

```json
{
  "method": "Emulation.setUserAgentOverride",
  "params": {
    "userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
    "acceptLanguage": "en-US,en;q=0.9",
    "platform": "MacIntel",
    "userAgentMetadata": {
      "brands": [
        {"brand": "Google Chrome", "version": "145"},
        {"brand": "Not:A-Brand",   "version": "99"},
        {"brand": "Chromium",      "version": "145"}
      ],
      "fullVersionList": [
        {"brand": "Google Chrome", "version": "145.0.7342.89"},
        {"brand": "Not:A-Brand",   "version": "99.0.0.0"},
        {"brand": "Chromium",      "version": "145.0.7342.89"}
      ],
      "fullVersion":     "145.0.7342.89",
      "platform":        "macOS",
      "platformVersion": "14.5.0",
      "architecture":    "arm",
      "model":           "",
      "mobile":          false,
      "bitness":         "64",
      "wow64":           false
    }
  }
}
```

---

## Gap 5c: SpeechSynthesis.getVoices()

Linux Chrome returns `[]` → unique leak. Real macOS 14 list (34 voices, from BotBrowser dumps):

```js
(() => {
  const REAL_MAC_VOICES = [
    ['Albert','en-US','com.apple.speech.synthesis.voice.Albert'],
    ['Alice','it-IT','com.apple.voice.compact.it-IT.Alice'],
    ['Alva','sv-SE','com.apple.voice.compact.sv-SE.Alva'],
    ['Amélie','fr-CA','com.apple.voice.compact.fr-CA.Amelie'],
    ['Anna','de-DE','com.apple.voice.compact.de-DE.Anna'],
    ['Carmit','he-IL','com.apple.voice.compact.he-IL.Carmit'],
    ['Daniel','en-GB','com.apple.voice.compact.en-GB.Daniel'],
    ['Fiona','en-GB','com.apple.voice.compact.en-scotland.Fiona'],
    ['Fred','en-US','com.apple.speech.synthesis.voice.Fred'],
    ['Karen','en-AU','com.apple.voice.compact.en-AU.Karen'],
    ['Kyoko','ja-JP','com.apple.voice.compact.ja-JP.Kyoko'],
    ['Luciana','pt-BR','com.apple.voice.compact.pt-BR.Luciana'],
    ['Maged','ar-SA','com.apple.voice.compact.ar-SA.Maged'],
    ['Mei-Jia','zh-TW','com.apple.voice.compact.zh-TW.Mei-Jia'],
    ['Melina','el-GR','com.apple.voice.compact.el-GR.Melina'],
    ['Milena','ru-RU','com.apple.voice.compact.ru-RU.Milena'],
    ['Moira','en-IE','com.apple.voice.compact.en-IE.Moira'],
    ['Monica','es-ES','com.apple.voice.compact.es-ES.Monica'],
    ['Nora','nb-NO','com.apple.voice.compact.nb-NO.Nora'],
    ['Paulina','es-MX','com.apple.voice.compact.es-MX.Paulina'],
    ['Samantha','en-US','com.apple.voice.compact.en-US.Samantha'],
    ['Sara','da-DK','com.apple.voice.compact.da-DK.Sara'],
    ['Satu','fi-FI','com.apple.voice.compact.fi-FI.Satu'],
    ['Sin-ji','zh-HK','com.apple.voice.compact.zh-HK.Sin-ji'],
    ['Tessa','en-ZA','com.apple.voice.compact.en-ZA.Tessa'],
    ['Thomas','fr-FR','com.apple.voice.compact.fr-FR.Thomas'],
    ['Ting-Ting','zh-CN','com.apple.voice.compact.zh-CN.Ting-Ting'],
    ['Veena','en-IN','com.apple.voice.compact.en-IN.Veena'],
    ['Victoria','en-US','com.apple.speech.synthesis.voice.Victoria'],
    ['Xander','nl-NL','com.apple.voice.compact.nl-NL.Xander'],
    ['Yelda','tr-TR','com.apple.voice.compact.tr-TR.Yelda'],
    ['Yuna','ko-KR','com.apple.voice.compact.ko-KR.Yuna'],
    ['Zosia','pl-PL','com.apple.voice.compact.pl-PL.Zosia'],
    ['Zuzana','cs-CZ','com.apple.voice.compact.cs-CZ.Zuzana'],
  ];
  const mkVoice = ([name, lang, uri]) => Object.freeze({
    default: name === 'Samantha',
    lang, localService: true, name, voiceURI: uri,
  });
  const voices = REAL_MAC_VOICES.map(mkVoice);
  SpeechSynthesis.prototype.getVoices = function() { return voices.slice(); };
  setTimeout(() => {
    if (typeof speechSynthesis.onvoiceschanged === 'function') {
      speechSynthesis.onvoiceschanged(new Event('voiceschanged'));
    }
  }, 100);
})();
```

---

## Cross-Cutting: Realm Propagation

| Fix | Main world | Isolated | Workers | iframes (OOPIF) |
|---|---|---|---|---|
| WebRTC wrap | Y | Y | Y (SW/dedicated) | Y |
| Timezone/Locale (CDP) | auto | auto | auto | Y (per-target) |
| navigator.languages shim | Y | N | Y | Y |
| Fonts shim | Y | N | N | Y |
| Audio | C++ (all) | — | — | — |
| Plugins | Y | N | N | Y |
| UA metadata (CDP) | auto | auto | NOT SW | Y (per-target) |
| SpeechSynthesis | Y | N | N | Y |

**Attachment recipe**:
```
Target.setDiscoverTargets {discover:true}
Target.setAutoAttach {autoAttach:true, waitForDebuggerOnStart:true, flatten:true}
// On Target.attachedToTarget(sessionId):
Emulation.setTimezoneOverride ...
Emulation.setLocaleOverride ...
Emulation.setUserAgentOverride ...
Emulation.setGeolocationOverride ...
Page.addScriptToEvaluateOnNewDocument { source: BUNDLED_STEALTH_JS, runImmediately: true, worldName: "" }
Page.addScriptToEvaluateOnNewDocument { source: BUNDLED_STEALTH_JS, runImmediately: true, worldName: "__stealth_iso" }
Runtime.runIfWaitingForDebugger
```

`runImmediately:true` is critical — otherwise stealth loses race vs detection probes on initial load.

---

## Prioritization

| # | Gap | Impact | Ref impl | Effort | Priority |
|---|---|---|---|---|---|
| 1 | 5b sec-ch-ua userAgentMetadata | Cloudflare/DataDome instant flag | CDP native | S | **P0** |
| 2 | 2 Timezone/Locale CDP | CreepJS lies, DataDome IP-TZ | CDP native | S | **P0** |
| 3 | 1 WebRTC IP leak | Cloudflare Ent/DataDome/browserleaks | CloakBrowser C++ | S flag / M JS | **P0** |
| 4 | 5a navigator.plugins | Sannysoft, BotD | puppeteer-extra-stealth | S | **P1** |
| 5 | 5c SpeechSynthesis voices | CreepJS, BotBrowser | profile table | M | **P1** |
| 6 | 3 Fonts | CreepJS platformClassifier | font pack + shim | M | **P1** |
| 7 | 4 Audio | (verified via CloakBrowser C++) | — | — | **P2** (verify only) |

---

## Self-Test Endpoint

Targets:
- `https://abrahamjuliot.github.io/creepjs/` → `#fingerprint-data` → lies count, trust score, per-section hashes (fonts, webrtc, audio, voices, ua)
- `https://bot.sannysoft.com/` → pass/fail per check (webdriver, chrome, plugins, permissions, languages, webGL)
- `https://rebrowser.net/bot-detector` → `window.botDetectorResults`
- `https://fingerprintjs.github.io/BotD/main/` → `BotD.detect()` result
- `https://browserleaks.com/webrtc` → IP leak
- `https://browserleaks.com/canvas` → canvas hash + uniqueness
- `https://pixelscan.net/` → JSON API `/api/v1/fp`

Endpoint: `POST /selftest?target=creepjs&profile=macos13_ru` → structured JSON report with trust_score, lies, sections, duration_ms. Schedule via dozor nightly, alert on `trust_score < 90` or new lies entries. Store history in Postgres.
