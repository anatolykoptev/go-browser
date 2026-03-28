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

  // --- Worker scope ---
  if (typeof WorkerGlobalScope !== 'undefined') {
    Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
  }
})();
