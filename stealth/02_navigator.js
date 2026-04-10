// Navigator property alignment — ensure main thread matches worker bootstrap.
// We do NOT proxy Function.prototype.toString (CreepJS hasToStringProxy).
// navigator.webdriver — CloakBrowser C++ handles it at C++ level.
// For non-CloakBrowser Chrome, we apply a JS fallback below.
// We ONLY set properties that the worker bootstrap also sets, to prevent
// bot.incolumitas.com inconsistentWebWorkerNavigatorPropery detection.

const __sp = window.__sp || {};

// navigator.webdriver — CloakBrowser C++ sets it to false in main thread.
// For non-CloakBrowser Chrome (direct rod/CDP), we override via JS.
// Use Object.defineProperty with value (not getter) to avoid lieProps detection.
if (navigator.webdriver !== false) {
  Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
    value: false,
    writable: true,
    configurable: true,
    enumerable: true,
  });
}

// deviceMemory — worker gets it from profile (default 8), main must match.
if (__sp.hardware && __sp.hardware.deviceMemory) {
  Object.defineProperty(Navigator.prototype, 'deviceMemory', {
    get: () => __sp.hardware.deviceMemory,
    configurable: true, enumerable: true,
  });
}

// languages — worker gets profile.languages (e.g. ["en-US","en"]),
// but main gets CDP Accept-Language which adds ";q=0.9" quality values.
// Override main to match worker's clean array from profile.
if (__sp.languages && __sp.languages.length) {
  const frozenLangs = Object.freeze(__sp.languages.slice());
  Object.defineProperty(Navigator.prototype, 'languages', {
    get: () => frozenLangs,
    configurable: true, enumerable: true,
  });
}
