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
  Object.defineProperty(navigator, 'webdriver', {get: () => undefined});

  // --- Notification.permission ---
  // Headless Chrome returns 'denied'; real browsers default to 'default'
  if (typeof Notification !== 'undefined') {
    Object.defineProperty(Notification, 'permission', {
      get: () => 'default',
      configurable: true,
    });
  }

  // --- chrome.runtime stub ---
  if (!window.chrome) window.chrome = {};
  if (!window.chrome.runtime) {
    window.chrome.runtime = {
      connect: () => {}, sendMessage: () => {},
      onMessage: {addListener: () => {}, removeListener: () => {}},
      id: undefined,
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

  // --- Worker scope ---
  if (typeof WorkerGlobalScope !== 'undefined') {
    Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
  }
})();
