// Navigator property alignment — ensure main thread matches worker bootstrap.
// We do NOT proxy Function.prototype.toString (CreepJS hasToStringProxy).
// We do NOT override navigator.webdriver (CloakBrowser C++ handles it).
// We ONLY set properties that the worker bootstrap also sets, to prevent
// bot.incolumitas.com inconsistentWebWorkerNavigatorPropery detection.

const __sp = window.__sp || {};

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
