(() => {

  // === 00_profile.js ===
  // Profile loader — reads window.__stealthProfile set by Go via EvalOnNewDocument.
  // All other stealth modules use window.__sp as shorthand.
  // If Go didn't inject a profile, the stealth complement won't apply profile-dependent overrides.
  if (window.__stealthProfile) {
    window.__sp = window.__stealthProfile;
  }

  // === 01_cdp_markers.js ===
  // CDP automation marker cleanup.
  // Removes rod/ChromeDriver/Playwright markers from window
  // and watches for dynamically injected marker attributes.
  
  const markerPatterns = [/^\$cdc_/, /^\$chrome_/, /^__webdriver/, /^__selenium/, /^__playwright/, /^__pw_/];
  for (const key of Object.keys(window)) {
    if (markerPatterns.some(p => p.test(key))) {
      try { delete window[key]; } catch(e) {}
    }
  }
  try { delete window.__cdp_runtime; } catch(e) {}
  
  const obs = new MutationObserver(mutations => {
    for (const m of mutations) {
      if (m.type === 'attributes' && markerPatterns.some(p => p.test(m.attributeName))) {
        document.documentElement.removeAttribute(m.attributeName);
      }
    }
  });
  if (document.documentElement) {
    obs.observe(document.documentElement, { attributes: true });
  }
  
  // Prevent stack-based CDP detection via Error.prepareStackTrace setter.
  const origPST = Error.prepareStackTrace;
  Object.defineProperty(Error, 'prepareStackTrace', {
    get: () => origPST,
    set: () => {},
    configurable: false,
  });
  
  // Block localhost port scanning (PerimeterX, eBay use this)
  (function() {
    var origFetch = window.fetch;
    window.fetch = function(url) {
      if (typeof url === 'string' && /^https?:\/\/(localhost|127\.|0\.0\.0\.0|\[::1\])/.test(url)) {
        return Promise.reject(new TypeError('Failed to fetch'));
      }
      return origFetch.apply(this, arguments);
    };
    var OrigWebSocket = window.WebSocket;
    window.WebSocket = function(url) {
      if (/^wss?:\/\/(localhost|127\.|0\.0\.0\.0|\[::1\])/.test(url)) {
        throw new DOMException("Failed to construct 'WebSocket'", 'SecurityError');
      }
      return new OrigWebSocket(url, arguments[1]);
    };
    window.WebSocket.prototype = OrigWebSocket.prototype;
    window.WebSocket.CONNECTING = 0;
    window.WebSocket.OPEN = 1;
    window.WebSocket.CLOSING = 2;
    window.WebSocket.CLOSED = 3;
  })();

  // === 02_navigator.js ===
  // Native toString masking — make overridden getters look like [native code]
  (function() {
    const _toString = Function.prototype.toString;
    const _nativeMap = new WeakMap();
  
    Function.prototype.toString = function() {
      const native = _nativeMap.get(this);
      if (native) return native;
      return _toString.call(this);
    };
  
    // Helper: define a property with a getter that reports as native code
    window.__defineNativeGetter = function(obj, prop, getter, nativeName) {
      _nativeMap.set(getter, 'function get ' + (nativeName || prop) + '() { [native code] }');
      Object.defineProperty(obj, prop, {
        get: getter,
        configurable: true,
        enumerable: true
      });
    };
  
    // Also mask Function.prototype.toString itself
    _nativeMap.set(Function.prototype.toString, 'function toString() { [native code] }');
  })();
  
  // Navigator property overrides — all values from active stealth profile.
  const sp = window.__sp;
  
  if (sp) {
    // webdriver = false (not undefined)
    window.__defineNativeGetter(Object.getPrototypeOf(navigator), 'webdriver', () => false);
  
    // userAgentData from profile
    if (!navigator.userAgentData && sp.userAgentData) {
      const uad = sp.userAgentData;
      const fvl = uad.fullVersionList || uad.brands.map(b => ({...b}));
      window.__defineNativeGetter(navigator, 'userAgentData', () => ({
        brands: uad.brands,
        mobile: uad.mobile,
        platform: uad.platform,
        getHighEntropyValues: (hints) => Promise.resolve({
          brands: uad.brands,
          mobile: uad.mobile,
          platform: uad.platform,
          platformVersion: uad.platformVersion,
          architecture: uad.architecture,
          bitness: uad.bitness,
          model: '',
          uaFullVersion: uad.fullVersion,
          fullVersionList: fvl,
        }),
        toJSON: () => ({brands: uad.brands, mobile: uad.mobile, platform: uad.platform}),
      }));
    }
  
    // Hardware from profile
    if (sp.hardware) {
      window.__defineNativeGetter(Navigator.prototype, 'hardwareConcurrency',
        () => sp.hardware.hardwareConcurrency);
      window.__defineNativeGetter(Navigator.prototype, 'deviceMemory',
        () => sp.hardware.deviceMemory);
      window.__defineNativeGetter(Navigator.prototype, 'maxTouchPoints',
        () => sp.hardware.maxTouchPoints);
    }
  
    // Languages from profile
    if (sp.languages) {
      window.__defineNativeGetter(Navigator.prototype, 'languages',
        () => Object.freeze([...sp.languages]));
      window.__defineNativeGetter(Navigator.prototype, 'language',
        () => sp.languages[0]);
    }
  
    // Screen from profile
    if (sp.screen) {
      const s = sp.screen;
      for (const [k, v] of Object.entries(s)) {
        if (k === 'devicePixelRatio') {
          window.__defineNativeGetter(window, 'devicePixelRatio', () => v);
        } else {
          window.__defineNativeGetter(screen, k, () => v);
        }
      }
    }
  
    // GPU — WebGL vendor/renderer spoofing from profile
    if (sp.gpu) {
      const spoofWebGL = (proto) => {
        const orig = proto.getParameter;
        proto.getParameter = function(param) {
          if (param === 37445) return sp.gpu.vendor;
          if (param === 37446) return sp.gpu.renderer;
          return orig.apply(this, arguments);
        };
      };
      spoofWebGL(WebGLRenderingContext.prototype);
      if (typeof WebGL2RenderingContext !== 'undefined') {
        spoofWebGL(WebGL2RenderingContext.prototype);
      }
    }
  
    // NetworkInformation API from profile
    if (sp.connection && 'connection' in navigator) {
      const conn = sp.connection;
      const connProxy = {};
      for (const [k, v] of Object.entries(conn)) {
        window.__defineNativeGetter(connProxy, k, () => v);
      }
      connProxy.addEventListener = function() {};
      connProxy.removeEventListener = function() {};
      connProxy.onchange = null;
      window.__defineNativeGetter(navigator, 'connection', () => connProxy);
    }
  
    // mediaDevices stub
    if (!navigator.mediaDevices) {
      window.__defineNativeGetter(navigator, 'mediaDevices', () => ({
        enumerateDevices: () => Promise.resolve([
          {deviceId: '', groupId: '', kind: 'audioinput', label: ''},
          {deviceId: '', groupId: '', kind: 'videoinput', label: ''},
          {deviceId: '', groupId: '', kind: 'audiooutput', label: ''},
        ]),
        getUserMedia: () => Promise.reject(new DOMException('Permission denied')),
      }));
    }
  
    // document.hasFocus — headless returns false, real browser returns true
    document.hasFocus = function() { return true; };
    document.hasFocus.toString = function() { return 'function hasFocus() { [native code] }'; };
  
    // outerWidth/outerHeight — headless returns 0, real browser matches window
    if (sp.screen) {
      Object.defineProperty(window, 'outerWidth', {
        get: () => sp.screen.width, configurable: true
      });
      Object.defineProperty(window, 'outerHeight', {
        get: () => sp.screen.height + 77, configurable: true // 77px = title+toolbar on macOS
      });
      Object.defineProperty(window, 'screenX', {get: () => 0, configurable: true});
      Object.defineProperty(window, 'screenY', {get: () => 25, configurable: true}); // below menu bar
    }
  
    // navigator.platform from profile
    if (sp.platform) {
      window.__defineNativeGetter(Navigator.prototype, 'platform', () => sp.platform);
    }
  
    // Battery API fallback for environments where it's not available
    if (typeof navigator.getBattery !== 'function') {
      navigator.getBattery = function() {
        return Promise.resolve({
          charging: true, chargingTime: 0, dischargingTime: Infinity,
          level: 0.87 + Math.random() * 0.1,
          addEventListener: function() {}, removeEventListener: function() {}
        });
      };
    }
  
    // Gamepad API — real Chrome returns [null, null, null, null]
    if (typeof navigator.getGamepads !== 'function') {
      navigator.getGamepads = function() { return [null, null, null, null]; };
    }
  }

  // === 03_chrome_object.js ===
  // Chrome object stubs — Castle.io and other detectors check these.
  // Headless Chrome has incomplete window.chrome; real Chrome has all of these.
  
  if (!window.chrome) window.chrome = {};
  
  if (!window.chrome.runtime) {
    window.chrome.runtime = {
      connect: () => ({
        name: '', sender: undefined,
        onDisconnect: {addListener(){}, removeListener(){}, hasListener(){return false}, hasListeners(){return false}},
        onMessage: {addListener(){}, removeListener(){}, hasListener(){return false}, hasListeners(){return false}},
        postMessage(){}, disconnect(){}
      }),
      sendMessage: () => {},
      onMessage: {addListener: () => {}, removeListener: () => {}},
      id: undefined,
    };
  }
  
  if (!window.chrome.csi) {
    window.chrome.csi = function() {
      var t = performance.timing || {};
      var navStart = t.navigationStart || (Date.now() - 5000);
      return {
        startE: navStart,
        onloadT: (t.loadEventEnd || navStart + 2000),
        pageT: performance.now(),
        tran: 15
      };
    };
  }
  
  if (!window.chrome.loadTimes) {
    window.chrome.loadTimes = function() {
      var t = performance.timing || {};
      var navStart = (t.navigationStart || Date.now() - 5000) / 1000;
      return {
        requestTime: navStart,
        startLoadTime: navStart + 0.1,
        commitLoadTime: navStart + 0.3,
        finishDocumentLoadTime: navStart + 1.2,
        finishLoadTime: navStart + 1.5,
        firstPaintTime: navStart + 0.8,
        firstPaintAfterLoadTime: 0,
        navigationType: 'Other',
        wasFetchedViaSpdy: true,
        wasNpnNegotiated: true,
        npnNegotiatedProtocol: 'h2',
        wasAlternateProtocolAvailable: false,
        connectionInfo: 'h2'
      };
    };
  }
  
  if (!window.chrome.app) {
    window.chrome.app = {
      isInstalled: false,
      InstallState: {DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed'},
      RunningState: {CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running'},
      getDetails() {return null}, getIsInstalled() {return false}
    };
  }

  // === 04_media_permissions.js ===
  // Media codecs, notifications, and permissions overrides.
  
  // Video codec support — headless may report different support.
  const origCPT = HTMLMediaElement.prototype.canPlayType;
  HTMLMediaElement.prototype.canPlayType = function(type) {
    if (type.includes('h264') || type.includes('avc1')) return 'probably';
    if (type.includes('vp8') || type.includes('vp9')) return 'probably';
    return origCPT.call(this, type);
  };
  
  // Notification.permission — headless returns 'denied', real browsers default to 'default'.
  if (typeof Notification !== 'undefined') {
    Object.defineProperty(Notification, 'permission', {
      get: () => 'default',
      configurable: true,
    });
  }
  
  // Permissions.query — headless returns 'denied' for notifications.
  if (typeof Permissions !== 'undefined') {
    const origQuery = Permissions.prototype.query;
    Permissions.prototype.query = function(desc) {
      if (desc.name === 'notifications') {
        return Promise.resolve({state: Notification.permission});
      }
      return origQuery.apply(this, arguments);
    };
  }

  // === 05_worker_injection.js ===
  // Worker thread injection — patches navigator in all Worker contexts.
  // Handles string URLs, blob: URLs, and data: URLs.
  // Profile values are read from window.__sp once (at main-page eval time)
  // and serialised as a single JSON blob embedded in the bootstrap code.
  // This avoids template-literal sprawl and makes adding new profile fields
  // a zero-change operation here.
  
  const OriginalWorker = Worker;
  
  // Serialise the full profile once.  Workers receive it as PROFILE constant.
  const _workerProfile = (() => {
    const sp = window.__sp || {};
    return JSON.stringify({
      hardwareConcurrency: (sp.hardware || {}).hardwareConcurrency || 8,
      deviceMemory:        (sp.hardware || {}).deviceMemory        || 8,
      maxTouchPoints:      (sp.hardware || {}).maxTouchPoints      || 0,
      platform:            sp.platform  || 'MacIntel',
      languages:           sp.languages || ['en-US', 'en'],
      userAgent:           sp.userAgent || navigator.userAgent,
    });
  })();
  
  const workerBootstrap = [
    'const PROFILE = ' + _workerProfile + ';',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "webdriver", {',
    '  get: () => false, configurable: true, enumerable: true',
    '});',
    'Object.defineProperty(Navigator.prototype, "hardwareConcurrency", {',
    '  get: () => PROFILE.hardwareConcurrency, configurable: true',
    '});',
    'Object.defineProperty(Navigator.prototype, "deviceMemory", {',
    '  get: () => PROFILE.deviceMemory, configurable: true',
    '});',
    'Object.defineProperty(Navigator.prototype, "maxTouchPoints", {',
    '  get: () => PROFILE.maxTouchPoints, configurable: true',
    '});',
    'Object.defineProperty(Navigator.prototype, "platform", {',
    '  get: () => PROFILE.platform, configurable: true',
    '});',
    'Object.defineProperty(Navigator.prototype, "languages", {',
    '  get: () => Object.freeze(PROFILE.languages.slice()), configurable: true',
    '});',
    'Object.defineProperty(Navigator.prototype, "language", {',
    '  get: () => PROFILE.languages[0], configurable: true',
    '});',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "userAgent", {',
    '  get: () => PROFILE.userAgent, configurable: true',
    '});',
  ].join('\n');
  
  function createPatchedWorker(originalUrl, options) {
    // For blob: and data: URLs we cannot fetch them.
    // Create a new blob that imports the original.
    if (typeof originalUrl === 'string' &&
        (originalUrl.startsWith('blob:') || originalUrl.startsWith('data:'))) {
      try {
        var code = workerBootstrap + '\nimportScripts("' + originalUrl + '");';
        var blob = new Blob([code], {type: 'application/javascript'});
        var blobUrl = URL.createObjectURL(blob);
        return new OriginalWorker(blobUrl, options);
      } catch(e) {
        return new OriginalWorker(originalUrl, options);
      }
    }
  
    // For regular URLs, fetch + prepend bootstrap.
    try {
      var pending = [];
      var real = null;
      var handlers = {};
  
      fetch(originalUrl).then(function(r) { return r.text(); }).then(function(code) {
        var blob = new Blob([workerBootstrap + '\n' + code], {type: 'application/javascript'});
        var w = new OriginalWorker(URL.createObjectURL(blob), options);
        real = w;
        pending.forEach(function(m) { w.postMessage(m); });
        pending = null;
        if (handlers.message) w.onmessage = handlers.message;
        if (handlers.error)   w.onerror   = handlers.error;
      }).catch(function() {
        real = new OriginalWorker(originalUrl, options);
        if (pending) { pending.forEach(function(m) { real.postMessage(m); }); pending = null; }
        if (handlers.message) real.onmessage = handlers.message;
      });
  
      return {
        postMessage: function(msg) { if (real) real.postMessage(msg); else pending.push(msg); },
        terminate:   function()    { if (real) real.terminate(); },
        set onmessage(fn) { if (real) real.onmessage = fn; else handlers.message = fn; },
        get onmessage()   { return real ? real.onmessage : handlers.message; },
        set onerror(fn)   { if (real) real.onerror = fn; else handlers.error = fn; },
        get onerror()     { return real ? real.onerror : handlers.error; },
        addEventListener: function() {
          var args = arguments;
          if (real) real.addEventListener.apply(real, args);
          else setTimeout(function() { if (real) real.addEventListener.apply(real, args); }, 100);
        },
        removeEventListener: function() {
          if (real) real.removeEventListener.apply(real, arguments);
        },
        dispatchEvent: function(e) { if (real) return real.dispatchEvent(e); return false; },
      };
    } catch(e) {
      return new OriginalWorker(originalUrl, options);
    }
  }
  
  window.Worker = function(url, options) {
    return createPatchedWorker(url, options);
  };
  
  // Mask Worker.toString() to look native.
  window.Worker.toString = function() { return 'function Worker() { [native code] }'; };
  
  // Preserve constructor identity.
  Object.defineProperty(window.Worker, 'prototype', {
    value: OriginalWorker.prototype,
    writable: false,
    configurable: false,
  });
  
  // Clean up stealth markers.
  delete window.__stealthProfile;
  delete window.__sp;
  delete window.__defineNativeGetter;

  // === 06_webrtc_leak.js ===
  // WebRTC local IP leak prevention.
  // Wraps RTCPeerConnection (and legacy webkitRTCPeerConnection) to:
  //   1. Strip STUN servers from ICE config — no mDNS/RFC1918 gathering.
  //   2. Filter .local mDNS and RFC1918 candidates from icecandidate events.
  // Preserves prototype chain and masks toString to pass native-code checks.
  
  (() => {
    const RFC1918 = /\b(10\.\d+\.\d+\.\d+|127\.\d+\.\d+\.\d+|192\.168\.\d+\.\d+|172\.(1[6-9]|2\d|3[01])\.\d+\.\d+)\b/;
    const MDNS    = /\.local\b/;
    const STUN    = /^stun:/i;
  
    const isPrivateCandidate = (candidateStr) =>
      MDNS.test(candidateStr) || RFC1918.test(candidateStr);
  
    const wrap = (OrigPC) => {
      if (typeof OrigPC !== 'function') return OrigPC;
  
      const Wrapped = function RTCPeerConnection(config, ...rest) {
        if (config && Array.isArray(config.iceServers)) {
          config = Object.assign({}, config, {
            iceServers: config.iceServers.filter(s => {
              const urls = [].concat(s.urls || s.url || []);
              return urls.every(u => !STUN.test(u));
            }),
          });
        }
  
        const pc = new OrigPC(config, ...rest);
  
        // Intercept addEventListener to filter icecandidate events.
        const origAdd = pc.addEventListener.bind(pc);
        pc.addEventListener = function(type, cb, ...opts) {
          if (type !== 'icecandidate' || typeof cb !== 'function') {
            return origAdd(type, cb, ...opts);
          }
          return origAdd(type, (ev) => {
            if (ev.candidate && ev.candidate.candidate &&
                isPrivateCandidate(ev.candidate.candidate)) {
              return; // drop private candidate
            }
            cb(ev);
          }, ...opts);
        };
  
        // Mirror onicecandidate setter through the filtered addEventListener.
        Object.defineProperty(pc, 'onicecandidate', {
          set(fn) { pc.addEventListener('icecandidate', fn); },
          get() { return null; },
          configurable: true,
        });
  
        return pc;
      };
  
      // Preserve prototype identity so instanceof checks pass.
      Wrapped.prototype = OrigPC.prototype;
      Object.setPrototypeOf(Wrapped, OrigPC);
      Wrapped.toString = () => OrigPC.toString();
  
      return Wrapped;
    };
  
    if (typeof window.RTCPeerConnection !== 'undefined') {
      window.RTCPeerConnection = wrap(window.RTCPeerConnection);
    }
    if (typeof window.webkitRTCPeerConnection !== 'undefined') {
      window.webkitRTCPeerConnection = wrap(window.webkitRTCPeerConnection);
    }
  })();

  // === 07_navigator_plugins.js ===
  // navigator.plugins and navigator.mimeTypes spoofing.
  // Chrome 145 always exposes 5 hardcoded PDF plugin entries (since Chrome 92+).
  // Profile-driven: reads plugin list from window.__sp.plugins if present,
  // otherwise falls back to the canonical 5-entry PDF set.
  
  (() => {
    const DEFAULT_PLUGINS = [
      {name: 'PDF Viewer',                filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
      {name: 'Chrome PDF Viewer',         filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
      {name: 'Chromium PDF Viewer',       filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
      {name: 'Microsoft Edge PDF Viewer', filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
      {name: 'WebKit built-in PDF',       filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
    ];
    const DEFAULT_MIMETYPES = [
      {type: 'application/pdf', suffixes: 'pdf', description: 'Portable Document Format'},
      {type: 'text/pdf',        suffixes: 'pdf', description: 'Portable Document Format'},
    ];
  
    const sp = window.__sp;
    const pluginData   = (sp && Array.isArray(sp.plugins) && sp.plugins.length > 0)
      ? sp.plugins : DEFAULT_PLUGINS;
  
    // Build MimeType-like objects with prototype preservation.
    const makeMimeType = (m) => {
      const mt = Object.create(MimeType.prototype);
      Object.defineProperties(mt, {
        type:        {value: m.type,        enumerable: true},
        suffixes:    {value: m.suffixes,    enumerable: true},
        description: {value: m.description, enumerable: true},
        enabledPlugin: {value: null,        enumerable: true},
      });
      return mt;
    };
  
    const mimeTypes = DEFAULT_MIMETYPES.map(makeMimeType);
  
    // Build Plugin-like objects. Each plugin exposes its two MIME types by index.
    const makePlugin = (d) => {
      const p = Object.create(Plugin.prototype);
      Object.defineProperties(p, {
        name:        {value: d.name,        enumerable: true},
        filename:    {value: d.filename,    enumerable: true},
        description: {value: d.description, enumerable: true},
        length:      {value: mimeTypes.length},
      });
      mimeTypes.forEach((mt, i) => { p[i] = mt; });
      p.item      = (i) => mimeTypes[i] || null;
      p.namedItem = (n) => mimeTypes.find(m => m.type === n) || null;
      return p;
    };
  
    const plugins = pluginData.map(makePlugin);
  
    // Assemble PluginArray.
    const pluginArray = Object.create(PluginArray.prototype);
    plugins.forEach((p, i) => { pluginArray[i] = p; });
    Object.defineProperty(pluginArray, 'length', {value: plugins.length});
    pluginArray.item      = (i) => plugins[i] || null;
    pluginArray.namedItem = (n) => plugins.find(p => p.name === n) || null;
    pluginArray.refresh   = () => {};
    pluginArray[Symbol.iterator] = function* () { yield* plugins; };
  
    // Assemble MimeTypeArray.
    const mimeTypeArray = Object.create(MimeTypeArray.prototype);
    mimeTypes.forEach((m, i) => { mimeTypeArray[i] = m; });
    Object.defineProperty(mimeTypeArray, 'length', {value: mimeTypes.length});
    mimeTypeArray.item      = (i) => mimeTypes[i] || null;
    mimeTypeArray.namedItem = (n) => mimeTypes.find(m => m.type === n) || null;
    mimeTypeArray[Symbol.iterator] = function* () { yield* mimeTypes; };
  
    Object.defineProperty(Navigator.prototype, 'plugins', {
      get: () => pluginArray,
      configurable: true,
    });
    Object.defineProperty(Navigator.prototype, 'mimeTypes', {
      get: () => mimeTypeArray,
      configurable: true,
    });
  })();

  // === 08_speech_voices.js ===
  // SpeechSynthesis.getVoices() spoofing.
  // Linux/Docker Chrome returns [] — a unique headless signal.
  // Profile-driven: reads voice list from window.__sp.voices if present.
  // Falls back to empty list for non-macOS profiles (realistic for Linux/Windows).
  
  (() => {
    const sp = window.__sp;
    const voiceData = (sp && Array.isArray(sp.voices)) ? sp.voices : [];
    if (voiceData.length === 0) return; // skip patching for profiles without voices
  
    const voices = voiceData.map(v => Object.freeze({
      default:      v.default === true,
      lang:         v.lang,
      localService: true,
      name:         v.name,
      voiceURI:     v.voiceURI,
    }));
  
    if (typeof SpeechSynthesis !== 'undefined') {
      SpeechSynthesis.prototype.getVoices = function() { return voices.slice(); };
    }
  
    // Fire voiceschanged so pages that wait for the event get real voices.
    setTimeout(() => {
      if (typeof window.speechSynthesis !== 'undefined' &&
          typeof window.speechSynthesis.onvoiceschanged === 'function') {
        window.speechSynthesis.onvoiceschanged(new Event('voiceschanged'));
      }
    }, 100);
  })();

  // === 09_fonts_shim.js ===
  // Font fingerprint shim — hides residual Linux-only fonts from detection.
  // CreepJS probes fonts via FontFaceSet.check() and FontFaceSet.forEach().
  // We intercept both to return false/skip for known Linux-exclusive fonts.
  // macOS-specific fonts (from profile.fonts) are not present on Linux Docker,
  // so the Dockerfile.cloakbrowser layer installs them; this shim only hides
  // Linux fonts that would betray the host OS when spoofing macOS.
  
  (() => {
    const HIDDEN_LINUX_FONTS = new Set([
      'Arimo', 'Chilanka', 'Cousine', 'Jomolhari',
      'Liberation Mono', 'Liberation Sans', 'Liberation Serif',
      'Ubuntu', 'Ubuntu Mono', 'Ubuntu Condensed',
      'DejaVu Sans', 'DejaVu Sans Mono', 'DejaVu Serif',
      'Noto Color Emoji', 'MONO',
    ]);
  
    // FontFaceSet.check() — returns false for hidden fonts
    const origCheck = FontFaceSet.prototype.check;
    FontFaceSet.prototype.check = function(font, text) {
      for (const name of HIDDEN_LINUX_FONTS) {
        if (font.includes('"' + name + '"') || font.includes("'" + name + "'")) {
          return false;
        }
      }
      return origCheck.call(this, font, text);
    };
  
    // FontFaceSet.forEach() — skip hidden font entries
    const origForEach = FontFaceSet.prototype.forEach;
    FontFaceSet.prototype.forEach = function(cb, thisArg) {
      return origForEach.call(this, function(ff, k, set) {
        if (HIDDEN_LINUX_FONTS.has(ff.family)) return;
        return cb.call(thisArg, ff, k, set);
      });
    };
  
    // FontFaceSet[Symbol.iterator] — skip hidden font entries
    const origValues = FontFaceSet.prototype[Symbol.iterator];
    if (origValues) {
      FontFaceSet.prototype[Symbol.iterator] = function* () {
        for (const ff of origValues.call(this)) {
          if (!HIDDEN_LINUX_FONTS.has(ff.family)) yield ff;
        }
      };
    }
  })();

})();
