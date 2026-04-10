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
  // NOTE: navigator.webdriver is intentionally NOT overridden here.
  // CloakBrowser's C++ engine patches webdriver at binary level — including in worker
  // contexts. Adding a JS override on top creates a detectable lie: CreepJS's
  // queryLies() checks Function.prototype.toString on the getter and detects
  // "() => false" vs the expected "[native code]" string.
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

// Block ServiceWorker and SharedWorker so fingerprinting scripts
// (e.g. CreepJS) fall through to the DedicatedWorker path, which
// is controlled by our window.Worker override above and receives
// the WebGL bootstrap with the correct spoofed GPU values.
//
// ServiceWorkers run in a separate global scope that does NOT
// inherit EvalOnNewDocument injections, so they always expose the
// real GPU. Blocking register() forces the fallback to the patched
// DedicatedWorker.
if (navigator.serviceWorker) {
  try {
    const origRegister = navigator.serviceWorker.register.bind(navigator.serviceWorker);
    Object.defineProperty(navigator.serviceWorker, 'register', {
      value: function(scriptURL, options) {
        // Reject — CreepJS catches this and falls through to SharedWorker/DedicatedWorker.
        return Promise.reject(new DOMException(
          'ServiceWorker registration failed',
          'SecurityError'
        ));
      },
      writable: true,
      configurable: true,
    });
  } catch (_) {}
}

// Block SharedWorker so CreepJS falls through to DedicatedWorker.
// SharedWorkers also run without EvalOnNewDocument injections.
if (typeof SharedWorker !== 'undefined') {
  const OriginalSharedWorker = SharedWorker;
  window.SharedWorker = function(url, options) {
    // Return an object whose hasConstructor check fails so CreepJS skips it.
    throw new Error('SharedWorker unavailable');
  };
  Object.defineProperty(window.SharedWorker, 'prototype', {
    value: OriginalSharedWorker.prototype,
    writable: false,
    configurable: false,
  });
}

// Note: stealth marker cleanup (delete window.__sp etc.) is done by 09_fonts_shim.js
// which runs last, so that all modules can still read window.__sp when they run.
