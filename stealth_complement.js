(() => {

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
  // Navigator property overrides for headless detection evasion.
  
  // webdriver must be false (not undefined).
  // Chrome with --disable-blink-features=AutomationControlled returns false.
  Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
    get: () => false, configurable: true, enumerable: true
  });
  
  // NavigatorUAData (Chrome Client Hints).
  // Headless Chrome lacks navigator.userAgentData — critical for Castle.io.
  // Platform must match CloakBrowser's --fingerprint-platform and GPU renderer.
  // CloakBrowser with SwiftShader reports "Intel Iris OpenGL Engine" = macOS GPU.
  if (!navigator.userAgentData) {
    const brands = [
      {brand: 'Chromium', version: '145'},
      {brand: 'Google Chrome', version: '145'},
      {brand: 'Not-A.Brand', version: '24'}
    ];
    Object.defineProperty(navigator, 'userAgentData', {
      get: () => ({
        brands: brands,
        mobile: false,
        platform: 'macOS',
        getHighEntropyValues: (hints) => Promise.resolve({
          brands: brands,
          mobile: false,
          platform: 'macOS',
          platformVersion: '14.5.0',
          architecture: 'arm',
          bitness: '64',
          model: '',
          uaFullVersion: '145.0.7632.159',
          fullVersionList: brands.map(b => ({...b})),
        }),
        toJSON: () => ({brands: brands, mobile: false, platform: 'macOS'}),
      }),
      configurable: true,
    });
  }
  
  // mediaDevices stub — headless Chrome lacks media devices.
  if (!navigator.mediaDevices) {
    Object.defineProperty(navigator, 'mediaDevices', {
      get: () => ({
        enumerateDevices: () => Promise.resolve([
          {deviceId: '', groupId: '', kind: 'audioinput', label: ''},
          {deviceId: '', groupId: '', kind: 'videoinput', label: ''},
          {deviceId: '', groupId: '', kind: 'audiooutput', label: ''},
        ]),
        getUserMedia: () => Promise.reject(new DOMException('Permission denied')),
      }),
      configurable: true,
    });
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
  // Worker thread injection — Castle.io checks navigator.webdriver inside Workers.
  // Wraps the Worker constructor to prepend stealth overrides to worker code.
  
  const OriginalWorker = Worker;
  const workerBootstrap = `
    Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
      get: () => false, configurable: true, enumerable: true
    });
    Object.defineProperty(Navigator.prototype, 'hardwareConcurrency', {
      get: () => 8, configurable: true
    });
  `;
  window.Worker = function(url, options) {
    try {
      const wP = fetch(url).then(r => r.text()).then(code => {
        const blob = new Blob([workerBootstrap + code], {type: 'application/javascript'});
        return new OriginalWorker(URL.createObjectURL(blob), options);
      });
      let real = null;
      const pending = [];
      wP.then(w => { real = w; pending.forEach(m => w.postMessage(m)); });
      return {
        postMessage(msg) { if (real) real.postMessage(msg); else pending.push(msg); },
        set onmessage(fn) { wP.then(w => w.onmessage = fn); },
        terminate() { wP.then(w => w.terminate()); },
        addEventListener(...args) { wP.then(w => w.addEventListener(...args)); },
        removeEventListener(...args) { wP.then(w => w.removeEventListener(...args)); },
      };
    } catch(e) {
      return new OriginalWorker(url, options);
    }
  };

  // === 06_castle_events.js ===
  // Castle.io behavioral event generation.
  // Castle SDK Part 3 captures DOM interaction events (mousemove, click, keydown, scroll).
  // CDP Input.dispatch* bypasses DOM event listeners, so Castle sees no human activity.
  // This module dispatches synthetic DOM events that Castle's listeners can capture.
  
  window.__castleWarmup = function(durationMs) {
    durationMs = durationMs || 3000;
    const start = Date.now();
    let eventCount = 0;
  
    // Random point within viewport
    const randX = () => Math.floor(Math.random() * (window.innerWidth - 100)) + 50;
    const randY = () => Math.floor(Math.random() * (window.innerHeight - 100)) + 50;
  
    return new Promise(resolve => {
      const interval = setInterval(() => {
        const elapsed = Date.now() - start;
        if (elapsed >= durationMs) {
          clearInterval(interval);
          resolve(eventCount);
          return;
        }
  
        const x = randX(), y = randY();
        const opts = {bubbles: true, cancelable: true, clientX: x, clientY: y, button: 0};
  
        // Mousemove — most important for Castle behavioral tracking
        document.dispatchEvent(new MouseEvent('mousemove', opts));
        eventCount++;
  
        // Occasional click (every ~500ms)
        if (Math.random() < 0.15) {
          document.dispatchEvent(new PointerEvent('pointerdown', opts));
          document.dispatchEvent(new MouseEvent('mousedown', opts));
          document.dispatchEvent(new PointerEvent('pointerup', opts));
          document.dispatchEvent(new MouseEvent('mouseup', opts));
          eventCount += 4;
        }
  
        // Occasional scroll
        if (Math.random() < 0.1) {
          window.dispatchEvent(new WheelEvent('wheel', {
            deltaY: (Math.random() - 0.5) * 200,
            bubbles: true
          }));
          eventCount++;
        }
  
        // Occasional keydown (focus events)
        if (Math.random() < 0.05) {
          document.dispatchEvent(new KeyboardEvent('keydown', {
            key: 'Tab', code: 'Tab', bubbles: true
          }));
          document.dispatchEvent(new KeyboardEvent('keyup', {
            key: 'Tab', code: 'Tab', bubbles: true
          }));
          eventCount += 2;
        }
      }, 50); // ~20 events/sec = 60+ events in 3 seconds
    });
  };

})();
