(() => {
  // CDP cleanup
  try { delete window.__cdp_runtime; } catch(e) {}
  Object.defineProperty(navigator, 'webdriver', {get: () => undefined});

  // chrome.runtime stub
  if (!window.chrome) window.chrome = {};
  if (!window.chrome.runtime) {
    window.chrome.runtime = {
      connect: () => {}, sendMessage: () => {},
      onMessage: {addListener: () => {}, removeListener: () => {}},
      id: undefined
    };
  }

  // Media codecs
  const orig = HTMLMediaElement.prototype.canPlayType;
  HTMLMediaElement.prototype.canPlayType = function(type) {
    if (type.includes('h264') || type.includes('avc1')) return 'probably';
    if (type.includes('vp8') || type.includes('vp9')) return 'probably';
    return orig.call(this, type);
  };

  // Worker patches
  if (typeof WorkerGlobalScope !== 'undefined') {
    Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
  }
})();
