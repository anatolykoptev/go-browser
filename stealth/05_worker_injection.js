// Worker thread injection — patches navigator in all Worker contexts.
// Handles string URLs, blob: URLs, and data: URLs.

const OriginalWorker = Worker;

const workerBootstrap = (function() {
  const sp = window.__sp;
  const hwc = sp?.hardware?.hardwareConcurrency || 8;
  const dm = sp?.hardware?.deviceMemory || 8;
  const platform = sp?.platform || 'MacIntel';
  const langs = sp?.languages ? JSON.stringify(sp.languages) : '["en-US","en"]';

  return `
    Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
      get: () => false, configurable: true, enumerable: true
    });
    Object.defineProperty(Navigator.prototype, 'hardwareConcurrency', {
      get: () => ${hwc}, configurable: true
    });
    Object.defineProperty(Navigator.prototype, 'deviceMemory', {
      get: () => ${dm}, configurable: true
    });
    Object.defineProperty(Navigator.prototype, 'platform', {
      get: () => '${platform}', configurable: true
    });
    Object.defineProperty(Navigator.prototype, 'languages', {
      get: () => Object.freeze(${langs}), configurable: true
    });
    Object.defineProperty(Navigator.prototype, 'language', {
      get: () => ${langs}[0], configurable: true
    });
  `;
})();

function createPatchedWorker(originalUrl, options) {
  // For blob: and data: URLs, we can't fetch them.
  // Instead, create a new blob that imports the original.
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

  // For regular URLs, fetch + prepend bootstrap
  try {
    var pending = [];
    var real = null;
    var handlers = {};

    fetch(originalUrl).then(function(r) { return r.text(); }).then(function(code) {
      var blob = new Blob([workerBootstrap + '\n' + code], {type: 'application/javascript'});
      var w = new OriginalWorker(URL.createObjectURL(blob), options);
      real = w;
      // Replay pending messages
      pending.forEach(function(m) { w.postMessage(m); });
      pending = null;
      // Attach saved handlers
      if (handlers.message) w.onmessage = handlers.message;
      if (handlers.error) w.onerror = handlers.error;
    }).catch(function() {
      real = new OriginalWorker(originalUrl, options);
      if (pending) { pending.forEach(function(m) { real.postMessage(m); }); pending = null; }
      if (handlers.message) real.onmessage = handlers.message;
    });

    return {
      postMessage: function(msg) { if (real) real.postMessage(msg); else pending.push(msg); },
      terminate: function() { if (real) real.terminate(); },
      set onmessage(fn) { if (real) real.onmessage = fn; else handlers.message = fn; },
      get onmessage() { return real ? real.onmessage : handlers.message; },
      set onerror(fn) { if (real) real.onerror = fn; else handlers.error = fn; },
      get onerror() { return real ? real.onerror : handlers.error; },
      addEventListener: function() {
        var args = arguments;
        if (real) real.addEventListener.apply(real, args);
        else setTimeout(function() { if (real) real.addEventListener.apply(real, args); }, 100);
      },
      removeEventListener: function() {
        if (real) real.removeEventListener.apply(real, arguments);
      },
      dispatchEvent: function(e) { if (real) return real.dispatchEvent(e); return false; }
    };
  } catch(e) {
    return new OriginalWorker(originalUrl, options);
  }
}

window.Worker = function(url, options) {
  return createPatchedWorker(url, options);
};

// Mask Worker.toString() to look native
window.Worker.toString = function() { return 'function Worker() { [native code] }'; };

// Preserve constructor identity
Object.defineProperty(window.Worker, 'prototype', {
  value: OriginalWorker.prototype,
  writable: false,
  configurable: false
});

// Clean up stealth markers
delete window.__stealthProfile;
delete window.__sp;
delete window.__defineNativeGetter;
