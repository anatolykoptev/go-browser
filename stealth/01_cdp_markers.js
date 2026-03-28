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
