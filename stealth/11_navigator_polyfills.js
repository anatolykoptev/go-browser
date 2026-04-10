// Navigator API polyfills — fill in APIs that headless Chrome lacks.
// Each missing API is a signal in CreepJS likeHeadless checks.

const sp = window.__sp;

// pdfViewerEnabled — headless Chrome lacks PDF viewer.
// Real macOS Chrome has PDF viewer built-in (returns true).
if (navigator.pdfViewerEnabled === false || navigator.pdfViewerEnabled === undefined) {
  Object.defineProperty(Navigator.prototype, 'pdfViewerEnabled', {
    get: () => true,
    configurable: true,
    enumerable: true,
  });
}

// Web Share API — present on macOS Chrome but not in headless.
if (!navigator.share) {
  Object.defineProperty(Navigator.prototype, 'share', {
    value: (data) => {
      // Real Chrome rejects with NotAllowedError when called without user gesture.
      return Promise.reject(new DOMException(
        'Failed to execute \'share\' on \'Navigator\': Must be handling a user gesture',
        'NotAllowedError'
      ));
    },
    writable: true,
    configurable: true,
    enumerable: true,
  });
}

if (!navigator.canShare) {
  Object.defineProperty(Navigator.prototype, 'canShare', {
    value: (data) => {
      if (!data) return false;
      return !!(data.url || data.text || data.title || data.files);
    },
    writable: true,
    configurable: true,
    enumerable: true,
  });
}

// Content Indexing API — Chrome 84+ on Android; also present in desktop Chrome.
// Headless is missing this; likeHeadless.noContentIndex checks its presence.
if (!('index' in ServiceWorkerRegistration.prototype)) {
  try {
    Object.defineProperty(ServiceWorkerRegistration.prototype, 'index', {
      get: () => null,
      configurable: true,
      enumerable: true,
    });
  } catch (_) {}
}

// Contacts Manager API — Chrome on Android; desktop Chrome 91+ also has it.
if (!navigator.contacts) {
  Object.defineProperty(Navigator.prototype, 'contacts', {
    get: () => ({
      getProperties: () => Promise.resolve(['name', 'email', 'tel']),
      select: () => Promise.reject(new DOMException(
        'Failed to execute \'select\' on \'ContactsManager\': API not available',
        'InvalidStateError'
      )),
    }),
    configurable: true,
    enumerable: true,
  });
}

// downlinkMax — NetworkInformation API attribute.
// Headless Chrome's connection object may lack downlinkMax.
if (sp && sp.connection && 'connection' in navigator) {
  const conn = navigator.connection;
  if (conn && !('downlinkMax' in conn)) {
    try {
      Object.defineProperty(conn, 'downlinkMax', {
        get: () => Infinity,
        configurable: true,
        enumerable: true,
      });
    } catch (_) {}
  }
}
