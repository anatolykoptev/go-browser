(() => {
  // --- CDP automation marker cleanup ---
  // Remove rod/ChromeDriver/Playwright markers from window
  const markerPatterns = [/^\$cdc_/, /^\$chrome_/, /^__webdriver/, /^__selenium/, /^__playwright/, /^__pw_/];
  for (const key of Object.keys(window)) {
    if (markerPatterns.some(p => p.test(key))) {
      try { delete window[key]; } catch(e) {}
    }
  }
  try { delete window.__cdp_runtime; } catch(e) {}

  // Watch for dynamically injected marker attributes on <html>
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

  // --- Error.prepareStackTrace protection ---
  // Prevents stack-based CDP detection via Error.prepareStackTrace setter
  const origPST = Error.prepareStackTrace;
  Object.defineProperty(Error, 'prepareStackTrace', {
    get: () => origPST,
    set: () => {},
    configurable: false,
  });

  // --- navigator.webdriver ---
  // Must be false (not undefined) — Chrome with --disable-blink-features=AutomationControlled
  // returns false. Castle.io checks this distinction.
  Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
    get: () => false, configurable: true, enumerable: true
  });

  // --- Notification.permission ---
  // Headless Chrome returns 'denied'; real browsers default to 'default'
  if (typeof Notification !== 'undefined') {
    Object.defineProperty(Notification, 'permission', {
      get: () => 'default',
      configurable: true,
    });
  }

  // --- chrome object stubs ---
  // Full chrome.runtime + chrome.csi/loadTimes/app — Castle.io checks these
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

  // --- Media codecs ---
  const origCPT = HTMLMediaElement.prototype.canPlayType;
  HTMLMediaElement.prototype.canPlayType = function(type) {
    if (type.includes('h264') || type.includes('avc1')) return 'probably';
    if (type.includes('vp8') || type.includes('vp9')) return 'probably';
    return origCPT.call(this, type);
  };

  // --- NavigatorUAData (Chrome Client Hints) ---
  // Headless Chrome lacks navigator.userAgentData — critical detection vector
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
        platform: 'Windows',
        getHighEntropyValues: (hints) => Promise.resolve({
          brands: brands,
          mobile: false,
          platform: 'Windows',
          platformVersion: '15.0.0',
          architecture: 'x86',
          bitness: '64',
          model: '',
          uaFullVersion: '145.0.7632.159',
          fullVersionList: brands.map(b => ({...b})),
        }),
        toJSON: () => ({brands: brands, mobile: false, platform: 'Windows'}),
      }),
      configurable: true,
    });
  }

  // --- navigator.mediaDevices stub ---
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

  // --- Permissions.query ---
  // Headless Chrome returns 'denied' for notifications; real Chrome respects Notification.permission
  if (typeof Permissions !== 'undefined') {
    const origQuery = Permissions.prototype.query;
    Permissions.prototype.query = function(desc) {
      if (desc.name === 'notifications') {
        return Promise.resolve({state: Notification.permission});
      }
      return origQuery.apply(this, arguments);
    };
  }

  // --- Worker thread injection ---
  // Castle.io checks navigator.webdriver inside Workers too
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
})();
