// Chrome object stubs — Castle.io and other detectors check these.
// Headless Chrome has incomplete window.chrome; real Chrome has all of these.

if (!window.chrome) window.chrome = {};

if (!window.chrome.runtime) {
  window.chrome.runtime = {
    connect: () => ({
      name: '', sender: undefined,
      onDisconnect: {addListener(){}, removeListener(){}, hasListener(){return false}, hasListeners(){return false}},
      onMessage: {addListener(){}, removeListener(){}, hasListener(){return false}, hasListeners(){return false}},
      postMessage(){}, disconnect(){}
    }),
    sendMessage: () => {},
    onMessage: {addListener: () => {}, removeListener: () => {}},
    id: undefined,
  };
}

if (!window.chrome.csi) {
  window.chrome.csi = () => {
    const now = Date.now();
    return {startE: now, onloadT: now, pageT: now, tran: 15};
  };
}

if (!window.chrome.loadTimes) {
  window.chrome.loadTimes = () => {
    const now = Date.now() / 1000;
    return {
      requestTime: now, startLoadTime: now, commitLoadTime: now,
      finishDocumentLoadTime: now, finishLoadTime: now, firstPaintTime: now,
      firstPaintAfterLoadTime: 0, navigationType: 'Other',
      wasFetchedViaSpdy: false, wasNpnNegotiated: false, npnNegotiatedProtocol: '',
      wasAlternateProtocolAvailable: false, connectionInfo: 'h2'
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
