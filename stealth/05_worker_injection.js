// Worker thread injection — Castle.io checks navigator.webdriver inside Workers.
// Wraps the Worker constructor to prepend stealth overrides to worker code.

const OriginalWorker = Worker;
const hwc = window.__sp?.hardware?.hardwareConcurrency || 8;
const workerBootstrap = `
  Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
    get: () => false, configurable: true, enumerable: true
  });
  Object.defineProperty(Navigator.prototype, 'hardwareConcurrency', {
    get: () => ${hwc}, configurable: true
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
