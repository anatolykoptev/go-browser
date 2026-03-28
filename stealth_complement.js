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
    window.chrome.csi = () => {
      const now = Date.now();
      return {startE: now, onloadT: now, pageT: now, tran: 15};
    };
  }
  
  if (!window.chrome.loadTimes) {
    window.chrome.loadTimes = () => {
      const now = Date.now() / 1000;
      return {
        requestTime: now, startLoadTime: now, commitLoadTime: now,
        finishDocumentLoadTime: now, finishLoadTime: now, firstPaintTime: now,
        firstPaintAfterLoadTime: 0, navigationType: 'Other',
        wasFetchedViaSpdy: false, wasNpnNegotiated: false, npnNegotiatedProtocol: '',
        wasAlternateProtocolAvailable: false, connectionInfo: 'h2'
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
  
  const OriginalWorker = Worker;
  
  const workerBootstrap = (function() {
    const sp = window.__sp;
    const hwc = sp?.hardware?.hardwareConcurrency || 8;
    const dm = sp?.hardware?.deviceMemory || 8;
    const platform = sp?.platform || 'MacIntel';
    const langs = sp?.languages ? JSON.stringify(sp.languages) : '["en-US","en"]';
  
    return `
      Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
        get: () => false, configurable: true, enumerable: true
      });
      Object.defineProperty(Navigator.prototype, 'hardwareConcurrency', {
        get: () => ${hwc}, configurable: true
      });
      Object.defineProperty(Navigator.prototype, 'deviceMemory', {
        get: () => ${dm}, configurable: true
      });
      Object.defineProperty(Navigator.prototype, 'platform', {
        get: () => '${platform}', configurable: true
      });
      Object.defineProperty(Navigator.prototype, 'languages', {
        get: () => Object.freeze(${langs}), configurable: true
      });
      Object.defineProperty(Navigator.prototype, 'language', {
        get: () => ${langs}[0], configurable: true
      });
    `;
  })();
  
  function createPatchedWorker(originalUrl, options) {
    // For blob: and data: URLs, we can't fetch them.
    // Instead, create a new blob that imports the original.
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
  
    // For regular URLs, fetch + prepend bootstrap
    try {
      var pending = [];
      var real = null;
      var handlers = {};
  
      fetch(originalUrl).then(function(r) { return r.text(); }).then(function(code) {
        var blob = new Blob([workerBootstrap + '\n' + code], {type: 'application/javascript'});
        var w = new OriginalWorker(URL.createObjectURL(blob), options);
        real = w;
        // Replay pending messages
        pending.forEach(function(m) { w.postMessage(m); });
        pending = null;
        // Attach saved handlers
        if (handlers.message) w.onmessage = handlers.message;
        if (handlers.error) w.onerror = handlers.error;
      }).catch(function() {
        real = new OriginalWorker(originalUrl, options);
        if (pending) { pending.forEach(function(m) { real.postMessage(m); }); pending = null; }
        if (handlers.message) real.onmessage = handlers.message;
      });
  
      return {
        postMessage: function(msg) { if (real) real.postMessage(msg); else pending.push(msg); },
        terminate: function() { if (real) real.terminate(); },
        set onmessage(fn) { if (real) real.onmessage = fn; else handlers.message = fn; },
        get onmessage() { return real ? real.onmessage : handlers.message; },
        set onerror(fn) { if (real) real.onerror = fn; else handlers.error = fn; },
        get onerror() { return real ? real.onerror : handlers.error; },
        addEventListener: function() {
          var args = arguments;
          if (real) real.addEventListener.apply(real, args);
          else setTimeout(function() { if (real) real.addEventListener.apply(real, args); }, 100);
        },
        removeEventListener: function() {
          if (real) real.removeEventListener.apply(real, arguments);
        },
        dispatchEvent: function(e) { if (real) return real.dispatchEvent(e); return false; }
      };
    } catch(e) {
      return new OriginalWorker(originalUrl, options);
    }
  }
  
  window.Worker = function(url, options) {
    return createPatchedWorker(url, options);
  };
  
  // Preserve constructor identity
  Object.defineProperty(window.Worker, 'prototype', {
    value: OriginalWorker.prototype,
    writable: false,
    configurable: false
  });
  
  // Clean up stealth markers
  delete window.__stealthProfile;
  delete window.__sp;
  delete window.__defineNativeGetter;

})();
