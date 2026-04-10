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
  //
  // We use a Proxy on Function.prototype.toString rather than replacing it with
  // a regular function.  A Proxy around the native toString:
  //   - has no .prototype (forwarded to native, which has none)
  //   - throws TypeError for new Proxy() (native toString is not a constructor)
  //   - passes all descriptor checks (get/set/enumerable come from native)
  //   - returns native-looking strings for our spoofed functions via apply trap
  //
  // This avoids the `lieProps['Function.toString']` detection in CreepJS which
  // fires when Function.prototype.toString is replaced with a regular function.
  (function() {
    const _nativeToString = Function.prototype.toString;
    const _nativeMap = new WeakMap();
  
    const _toStringProxy = new Proxy(_nativeToString, {
      apply(target, thisArg, args) {
        const native = _nativeMap.get(thisArg);
        if (native) return native;
        return Reflect.apply(target, thisArg, args);
      },
    });
  
    // Replace on the prototype — the Proxy wraps the native fn, so all structural
    // checks (typeof, prototype presence, descriptor) forward to the native.
    Object.defineProperty(Function.prototype, 'toString', {
      value: _toStringProxy,
      writable: true,
      configurable: true,
      enumerable: false,
    });
  
    // The proxy itself must also look native when probed.
    _nativeMap.set(_toStringProxy, 'function toString() { [native code] }');
  
    // Helper: define a property with a getter that reports as native code
    window.__defineNativeGetter = function(obj, prop, getter, nativeName) {
      _nativeMap.set(getter, 'function get ' + (nativeName || prop) + '() { [native code] }');
      Object.defineProperty(obj, prop, {
        get: getter,
        configurable: true,
        enumerable: true
      });
    };
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
    window.chrome.runtime = {};
  }
  
  // CreepJS hasBadChromeRuntime: checks 'prototype' in sendMessage/connect and
  // that `new fn()` throws TypeError.  Arrow functions naturally satisfy both:
  // they have no .prototype and throw TypeError when constructed.
  // We always override these (even if runtime already exists in headless Chrome)
  // because the native headless stubs are regular functions with .prototype.
  window.chrome.runtime.sendMessage = () => {};
  window.chrome.runtime.connect = () => ({
    name: '', sender: undefined,
    onDisconnect: {addListener(){}, removeListener(){}, hasListener(){return false}, hasListeners(){return false}},
    onMessage: {addListener(){}, removeListener(){}, hasListener(){return false}, hasListeners(){return false}},
    postMessage(){}, disconnect(){}
  });
  if (!window.chrome.runtime.onMessage) {
    window.chrome.runtime.onMessage = {addListener: () => {}, removeListener: () => {}};
  }
  if (window.chrome.runtime.id === undefined) {
    window.chrome.runtime.id = undefined;
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
      gpuVendor:           (sp.gpu || {}).vendor || '',
      gpuRenderer:         (sp.gpu || {}).renderer || '',
    });
  })();
  
  const workerBootstrap = [
    'const PROFILE = ' + _workerProfile + ';',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "webdriver", {',
    '  get: () => false, configurable: true, enumerable: true',
    '});',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "hardwareConcurrency", {',
    '  get: () => PROFILE.hardwareConcurrency, configurable: true',
    '});',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "deviceMemory", {',
    '  get: () => PROFILE.deviceMemory, configurable: true',
    '});',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "maxTouchPoints", {',
    '  get: () => PROFILE.maxTouchPoints, configurable: true',
    '});',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "platform", {',
    '  get: () => PROFILE.platform, configurable: true',
    '});',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "languages", {',
    '  get: () => Object.freeze(PROFILE.languages.slice()), configurable: true',
    '});',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "language", {',
    '  get: () => PROFILE.languages[0], configurable: true',
    '});',
    'Object.defineProperty(Object.getPrototypeOf(navigator), "userAgent", {',
    '  get: () => PROFILE.userAgent, configurable: true',
    '});',
    // WebGL GPU spoof in workers — CreepJS hasBadWebGL compares main vs worker GPU.
    // Workers can create OffscreenCanvas and call getParameter(UNMASKED_RENDERER_WEBGL).
    // We must return the same vendor/renderer as the main world.
    '(function() {',
    '  if (!PROFILE.gpuVendor && !PROFILE.gpuRenderer) return;',
    '  function spoofWebGL(proto) {',
    '    if (!proto) return;',
    '    var origGet = proto.getParameter;',
    '    proto.getParameter = function(param) {',
    '      if (param === 37445) return PROFILE.gpuVendor;',   // UNMASKED_VENDOR_WEBGL
    '      if (param === 37446) return PROFILE.gpuRenderer;', // UNMASKED_RENDERER_WEBGL
    '      return origGet.apply(this, arguments);',
    '    };',
    '  }',
    '  if (typeof WebGLRenderingContext !== "undefined") spoofWebGL(WebGLRenderingContext.prototype);',
    '  if (typeof WebGL2RenderingContext !== "undefined") spoofWebGL(WebGL2RenderingContext.prototype);',
    '})();',
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
  
      // Use Object.create(OriginalWorker.prototype) so that hasConstructor checks
      // (e.g. x.__proto__.constructor.name == 'Worker') pass correctly.
      var proxy = Object.create(OriginalWorker.prototype);
      proxy.postMessage = function(msg) { if (real) real.postMessage(msg); else pending.push(msg); };
      proxy.terminate   = function()    { if (real) real.terminate(); };
      Object.defineProperty(proxy, 'onmessage', {
        get: function() { return real ? real.onmessage : handlers.message; },
        set: function(fn) { if (real) real.onmessage = fn; else handlers.message = fn; },
        configurable: true,
      });
      Object.defineProperty(proxy, 'onerror', {
        get: function() { return real ? real.onerror : handlers.error; },
        set: function(fn) { if (real) real.onerror = fn; else handlers.error = fn; },
        configurable: true,
      });
      proxy.addEventListener = function() {
        var args = arguments;
        if (real) real.addEventListener.apply(real, args);
        else setTimeout(function() { if (real) real.addEventListener.apply(real, args); }, 100);
      };
      proxy.removeEventListener = function() {
        if (real) real.removeEventListener.apply(real, arguments);
      };
      proxy.dispatchEvent = function(e) { if (real) return real.dispatchEvent(e); return false; };
      return proxy;
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
  
  // Note: stealth marker cleanup (delete window.__sp etc.) is done by 09_fonts_shim.js
  // which runs last, so that all modules can still read window.__sp when they run.

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
    if (typeof Plugin === 'undefined' || typeof PluginArray === 'undefined') return;
  
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
  
    if (typeof SpeechSynthesis !== 'undefined' && SpeechSynthesis.prototype) {
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
  // Font fingerprint shim — accurate font detection for headless Chrome.
  //
  // Problem: headless Chrome's document.fonts.check() returns true for ALL fonts
  // (including random nonexistent names), and FontFace.load('local(...)') fails
  // with "network error" for every font. Both lie to CreepJS font detection.
  //
  // Fix: replace document.fonts.check() with a set-based implementation that
  // accurately returns true only for fonts that match the installed set
  // (Apple system fonts + common cross-platform fonts) and false for everything
  // else — including Linux-exclusive fonts that betray the host OS.
  //
  // This shim also handles final cleanup of window.__sp and stealth markers
  // because it runs last (alphabetically 09 > all others).
  
  (() => {
    // Linux-only fonts — must return false when spoofing macOS.
    const LINUX_FONTS = new Set([
      'Arimo', 'Chilanka', 'Cousine', 'Jomolhari',
      'Liberation Mono', 'Liberation Sans', 'Liberation Serif',
      'Ubuntu', 'Ubuntu Mono', 'Ubuntu Condensed',
      'DejaVu Sans', 'DejaVu Sans Mono', 'DejaVu Serif',
      'Noto Color Emoji', 'MONO',
    ]);
  
    // Common macOS / cross-platform fonts that headless Chrome has via our
    // Dockerfile Apple font layer + base Chrome font packages.
    // These are the fonts CreepJS probes from its MacOSFonts + common sets.
    const MAC_SYSTEM_FONTS = new Set([
      // Core system fonts (pre-installed in Chrome base image)
      'Arial', 'Arial Black', 'Arial Narrow', 'Arial Unicode MS',
      'Comic Sans MS', 'Courier New', 'Georgia', 'Impact',
      'Times New Roman', 'Trebuchet MS', 'Verdana', 'Webdings',
      // Apple macOS system fonts (installed via Dockerfile Apple font layer)
      'SF Pro', 'SF Pro Display', 'SF Pro Text', 'SF Pro Rounded',
      'SF Compact', 'SF Compact Display', 'SF Compact Text',
      'SF Mono', 'New York',
      'Helvetica', 'Helvetica Neue',
      'Apple SD Gothic Neo', 'Apple SD Gothic Neo ExtraBold',
      'Geneva',
      // macOS version-specific fonts (from CreepJS MacOSFonts)
      'Kohinoor Devanagari Medium', 'Luminari',
      'PingFang HK Light',
      'American Typewriter Semibold', 'Futura Bold',
      'SignPainter-HouseScript Semibold',
      'InaiMathi Bold',
      'Galvji', 'MuktaMahee Regular',
      'Noto Sans Gunjala Gondi Regular', 'Noto Sans Masaram Gondi Regular',
      'Noto Serif Yezidi Regular',
      'STIX Two Math Regular', 'STIX Two Text Regular',
      'Noto Sans Canadian Aboriginal Regular',
    ]);
  
    // Merge profile-specific fonts if available.
    const sp = window.__sp;
    const profileFonts = (sp && sp.fonts) ? sp.fonts : [];
  
    // Build the full allow-set.
    const ALLOWED = new Set([...MAC_SYSTEM_FONTS]);
    for (const f of profileFonts) ALLOWED.add(f);
  
    // Extract the font family name from a CSS font shorthand string.
    // e.g. '16px "SF Pro Display"' → 'SF Pro Display'
    //      "0px 'Helvetica Neue'"  → 'Helvetica Neue'
    //      '12px Arial'            → 'Arial'
    const extractFamily = (fontStr) => {
      const s = String(fontStr);
      // Quoted name: extract between quotes
      const quoted = s.match(/["']([^"']+)["']/);
      if (quoted) return quoted[1];
      // Unquoted: last token(s) after the size/style info
      const parts = s.trim().split(/\s+/);
      // Font shorthand: size is last numeric token; family follows.
      // Simple heuristic: return everything after last size-like token.
      const sizeIdx = parts.findIndex(p => /^\d/.test(p));
      if (sizeIdx >= 0 && sizeIdx + 1 < parts.length) {
        return parts.slice(sizeIdx + 1).join(' ');
      }
      return parts[parts.length - 1];
    };
  
    // Replace document.fonts.check with accurate implementation.
    if (document.fonts && typeof document.fonts.check === 'function') {
      document.fonts.check = function(font, text) {
        const family = extractFamily(font);
        if (LINUX_FONTS.has(family)) return false;
        if (ALLOWED.has(family)) return true;
        // Unknown font: return false (no random-font-returns-true headless bug).
        return false;
      };
    }
  
    // Replace document.fonts.forEach to skip hidden Linux fonts.
    if (document.fonts && typeof document.fonts.forEach === 'function') {
      const origForEach = document.fonts.forEach.bind(document.fonts);
      document.fonts.forEach = function(cb, thisArg) {
        return origForEach(function(ff, k, set) {
          if (LINUX_FONTS.has(ff.family)) return;
          return cb.call(thisArg, ff, k, set);
        });
      };
    }
  })();
  
  // Cleanup — runs last so all prior modules (06-08) can still read __sp.
  delete window.__stealthProfile;
  delete window.__sp;
  delete window.__defineNativeGetter;

  // === 10_storage.js ===
  // Storage estimate spoof — return macOS-realistic quota/usage values.
  //
  // Headless Chrome returns a tiny quota (container disk space) which
  // fingerprint detectors use to identify non-desktop environments.
  // Real macOS with 512GB SSD typically reports ~450-500GB quota with
  // 30-40% usage depending on the machine state.
  //
  // Spec numbers for mac_chrome145 profile:
  //   quota:  494384795648  (~460 GB — typical 512GB Mac after OS overhead)
  //   usage:  189654345216  (~176 GB — ~38% used, realistic for a work machine)
  
  (() => {
    if (!navigator.storage || typeof navigator.storage.estimate !== 'function') return;
  
    const MAC_QUOTA = 494384795648;
    const MAC_USAGE = 189654345216;
  
    const _origEstimate = navigator.storage.estimate.bind(navigator.storage);
  
    // Use Object.defineProperty so the replacement isn't enumerable and
    // doesn't add an own .prototype that lieProps['StorageManager.estimate'] would flag.
    Object.defineProperty(navigator.storage, 'estimate', {
      value: () => Promise.resolve({
        quota: MAC_QUOTA,
        usage: MAC_USAGE,
        usageDetails: {},
      }),
      writable: true,
      configurable: true,
      enumerable: true,
    });
  })();

  // === 11_navigator_polyfills.js ===
  // Navigator API polyfills — fill in APIs that headless Chrome lacks.
  // Each missing API is a signal in CreepJS likeHeadless checks.
  //
  // NOTE: This module runs inside the outer IIFE from build.sh.
  // Do NOT declare bare `const sp` here — other modules already declare it
  // in the same scope. Use window.__sp directly or wrap in a nested IIFE.
  
  (() => {
    // Web Share API — present on macOS Chrome but not in headless.
    if (!navigator.share) {
      Object.defineProperty(Navigator.prototype, 'share', {
        value: (data) => {
          // Real Chrome rejects with NotAllowedError when called without user gesture.
          return Promise.reject(new DOMException(
            'Failed to execute \'share\' on \'Navigator\': Must be handling a user gesture',
            'NotAllowedError'
          ));
        },
        writable: true,
        configurable: true,
        enumerable: true,
      });
    }
  
    if (!navigator.canShare) {
      Object.defineProperty(Navigator.prototype, 'canShare', {
        value: (data) => {
          if (!data) return false;
          return !!(data.url || data.text || data.title || data.files);
        },
        writable: true,
        configurable: true,
        enumerable: true,
      });
    }
  
    // Contacts Manager API — Chrome on Android; desktop Chrome 91+ also has it.
    if (!navigator.contacts) {
      Object.defineProperty(Navigator.prototype, 'contacts', {
        get: () => ({
          getProperties: () => Promise.resolve(['name', 'email', 'tel']),
          select: () => Promise.reject(new DOMException(
            'Failed to execute \'select\' on \'ContactsManager\': API not available',
            'InvalidStateError'
          )),
        }),
        configurable: true,
        enumerable: true,
      });
    }
  
    // Content Indexing API — Chrome 84+ on Android; also present in desktop Chrome.
    // Wrapped in try/catch as ServiceWorkerRegistration may not be defined (e.g. about:blank).
    try {
      if (typeof ServiceWorkerRegistration !== 'undefined' &&
          !('index' in ServiceWorkerRegistration.prototype)) {
        Object.defineProperty(ServiceWorkerRegistration.prototype, 'index', {
          get: () => null,
          configurable: true,
          enumerable: true,
        });
      }
    } catch (_) {}
  
    // downlinkMax — NetworkInformation API attribute.
    try {
      if ('connection' in navigator && navigator.connection &&
          !('downlinkMax' in navigator.connection)) {
        Object.defineProperty(navigator.connection, 'downlinkMax', {
          get: () => Infinity,
          configurable: true,
          enumerable: true,
        });
      }
    } catch (_) {}
  })();

  // === 12_iframe_proxy.js ===
  // iframe.contentWindow proxy fix — Target.setAutoAttach (Gap C) enables
  // the DevTools protocol to eagerly create browsing contexts for iframes
  // with srcdoc set, even when they are not yet appended to the DOM.
  // Chrome sets contentWindow via C++ internals as an OWN data property on
  // the iframe instance, bypassing JS set traps (Object.defineProperty).
  // Real Chrome returns null contentWindow for detached iframes.
  // CreepJS hasIframeProxy detects this discrepancy.
  //
  // Fix: wrap each new iframe in a Proxy. A Proxy get trap intercepts ALL
  // reads regardless of how the underlying property was stored (C++ or JS).
  // When contentWindow is read on a detached iframe, return null.
  
  (() => {
    const origCreateElement = Document.prototype.createElement;
  
    Document.prototype.createElement = function(tag, options) {
      const el = origCreateElement.call(this, tag, options);
      if (typeof tag !== 'string' || tag.toLowerCase() !== 'iframe') {
        return el;
      }
  
      const proxy = new Proxy(el, {
        get(target, prop, receiver) {
          if (prop === 'contentWindow') {
            // Read the actual value Chrome stored (may be own C++ property).
            const val = target.contentWindow;
            // Return null when the iframe is not in the document — matches
            // real Chrome behaviour for detached iframes without setAutoAttach.
            return target.isConnected ? val : null;
          }
          const val = Reflect.get(target, prop, receiver);
          // Bind functions so `this` inside them refers to the real element.
          if (typeof val === 'function') {
            return val.bind(target);
          }
          return val;
        },
        set(target, prop, value, receiver) {
          return Reflect.set(target, prop, value, target);
        },
      });
  
      return proxy;
    };
  })();

})();
