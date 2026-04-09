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
  });
})();

const workerBootstrap = [
  'const PROFILE = ' + _workerProfile + ';',
  'Object.defineProperty(Object.getPrototypeOf(navigator), "webdriver", {',
  '  get: () => false, configurable: true, enumerable: true',
  '});',
  'Object.defineProperty(Navigator.prototype, "hardwareConcurrency", {',
  '  get: () => PROFILE.hardwareConcurrency, configurable: true',
  '});',
  'Object.defineProperty(Navigator.prototype, "deviceMemory", {',
  '  get: () => PROFILE.deviceMemory, configurable: true',
  '});',
  'Object.defineProperty(Navigator.prototype, "maxTouchPoints", {',
  '  get: () => PROFILE.maxTouchPoints, configurable: true',
  '});',
  'Object.defineProperty(Navigator.prototype, "platform", {',
  '  get: () => PROFILE.platform, configurable: true',
  '});',
  'Object.defineProperty(Navigator.prototype, "languages", {',
  '  get: () => Object.freeze(PROFILE.languages.slice()), configurable: true',
  '});',
  'Object.defineProperty(Navigator.prototype, "language", {',
  '  get: () => PROFILE.languages[0], configurable: true',
  '});',
  'Object.defineProperty(Object.getPrototypeOf(navigator), "userAgent", {',
  '  get: () => PROFILE.userAgent, configurable: true',
  '});',
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

    return {
      postMessage: function(msg) { if (real) real.postMessage(msg); else pending.push(msg); },
      terminate:   function()    { if (real) real.terminate(); },
      set onmessage(fn) { if (real) real.onmessage = fn; else handlers.message = fn; },
      get onmessage()   { return real ? real.onmessage : handlers.message; },
      set onerror(fn)   { if (real) real.onerror = fn; else handlers.error = fn; },
      get onerror()     { return real ? real.onerror : handlers.error; },
      addEventListener: function() {
        var args = arguments;
        if (real) real.addEventListener.apply(real, args);
        else setTimeout(function() { if (real) real.addEventListener.apply(real, args); }, 100);
      },
      removeEventListener: function() {
        if (real) real.removeEventListener.apply(real, arguments);
      },
      dispatchEvent: function(e) { if (real) return real.dispatchEvent(e); return false; },
    };
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

// Clean up stealth markers.
delete window.__stealthProfile;
delete window.__sp;
delete window.__defineNativeGetter;
