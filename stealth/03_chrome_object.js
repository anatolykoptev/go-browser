// Chrome object stubs — Castle.io and other detectors check these.
// Headless Chrome has incomplete window.chrome; real Chrome has all of these.

if (!window.chrome) window.chrome = {};

if (!window.chrome.runtime) {
  window.chrome.runtime = {};
}

// CreepJS hasBadChromeRuntime: checks 'prototype' in sendMessage/connect and
// that `new fn()` throws TypeError.  Arrow functions naturally satisfy both:
// they have no .prototype and throw TypeError when constructed.
// We always override these (even if runtime already exists in headless Chrome)
// because the native headless stubs are regular functions with .prototype.
window.chrome.runtime.sendMessage = () => {};
window.chrome.runtime.connect = () => ({
  name: '', sender: undefined,
  onDisconnect: {addListener(){}, removeListener(){}, hasListener(){return false}, hasListeners(){return false}},
  onMessage: {addListener(){}, removeListener(){}, hasListener(){return false}, hasListeners(){return false}},
  postMessage(){}, disconnect(){}
});
if (!window.chrome.runtime.onMessage) {
  window.chrome.runtime.onMessage = {addListener: () => {}, removeListener: () => {}};
}
if (window.chrome.runtime.id === undefined) {
  window.chrome.runtime.id = undefined;
}

if (!window.chrome.csi) {
  window.chrome.csi = function() {
    var t = performance.timing || {};
    var navStart = t.navigationStart || (Date.now() - 5000);
    return {
      startE: navStart,
      onloadT: (t.loadEventEnd || navStart + 2000),
      pageT: performance.now(),
      tran: 15
    };
  };
}

if (!window.chrome.loadTimes) {
  window.chrome.loadTimes = function() {
    var t = performance.timing || {};
    var navStart = (t.navigationStart || Date.now() - 5000) / 1000;
    return {
      requestTime: navStart,
      startLoadTime: navStart + 0.1,
      commitLoadTime: navStart + 0.3,
      finishDocumentLoadTime: navStart + 1.2,
      finishLoadTime: navStart + 1.5,
      firstPaintTime: navStart + 0.8,
      firstPaintAfterLoadTime: 0,
      navigationType: 'Other',
      wasFetchedViaSpdy: true,
      wasNpnNegotiated: true,
      npnNegotiatedProtocol: 'h2',
      wasAlternateProtocolAvailable: false,
      connectionInfo: 'h2'
    };
  };
}

if (!window.chrome.app) {
  window.chrome.app = {
    isInstalled: false,
    InstallState: {DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed'},
    RunningState: {CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running'},
    getDetails() {return null}, getIsInstalled() {return false}
  };
}
