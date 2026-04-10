// Navigator API polyfills — fill in APIs that headless Chrome lacks.
// Each missing API is a signal in CreepJS likeHeadless checks.
//
// NOTE: This module runs inside the outer IIFE from build.sh.
// Do NOT declare bare `const sp` here — other modules already declare it
// in the same scope. Use window.__sp directly or wrap in a nested IIFE.

(() => {
  // pdfViewerEnabled — headless Chrome defaults to false; real Chrome sets true.
  // CreepJS likeHeadless checks this directly.
  if (!navigator.pdfViewerEnabled) {
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

  // Content Indexing API — Chrome 84+ on Android; also present in desktop Chrome.
  // Wrapped in try/catch as ServiceWorkerRegistration may not be defined (e.g. about:blank).
  try {
    if (typeof ServiceWorkerRegistration !== 'undefined' &&
        !('index' in ServiceWorkerRegistration.prototype)) {
      Object.defineProperty(ServiceWorkerRegistration.prototype, 'index', {
        get: () => null,
        configurable: true,
        enumerable: true,
      });
    }
  } catch (_) {}

  // downlinkMax — NetworkInformation API attribute.
  try {
    if ('connection' in navigator && navigator.connection &&
        !('downlinkMax' in navigator.connection)) {
      Object.defineProperty(navigator.connection, 'downlinkMax', {
        get: () => Infinity,
        configurable: true,
        enumerable: true,
      });
    }
  } catch (_) {}

  // hasKnownBgColor — headless Chrome renders CSS ActiveText as rgb(255,0,0).
  // Real macOS Chrome renders it as system accent color (varies, but never pure red).
  // Override getComputedStyle to return non-red for ActiveText-styled elements.
  const origGetComputedStyle = window.getComputedStyle;
  window.getComputedStyle = function(el, pseudo) {
    const style = origGetComputedStyle.call(window, el, pseudo);
    if (style && style.backgroundColor === 'rgb(255, 0, 0)') {
      const s = el?.style;
      if (s && /ActiveText/i.test(s.backgroundColor || s.cssText || '')) {
        return new Proxy(style, {
          get(target, prop) {
            if (prop === 'backgroundColor') return 'rgb(0, 0, 0)';
            const v = target[prop];
            return typeof v === 'function' ? v.bind(target) : v;
          }
        });
      }
    }
    return style;
  };

  // prefersLightColor — Xvfb defaults to light scheme, ~60% of macOS users
  // have dark mode. Returning false for light = dark mode user = more common.
  const origMatchMedia = window.matchMedia;
  window.matchMedia = function(query) {
    const mql = origMatchMedia.call(window, query);
    if (/prefers-color-scheme:\s*light/i.test(query)) {
      return Object.create(mql, {
        matches: { get: () => false, configurable: true },
      });
    }
    return mql;
  };

  // ContentIndex — Chrome 84+, CreepJS checks window-level constructor.
  if (typeof window.ContentIndex === 'undefined') {
    window.ContentIndex = class ContentIndex {
      async add() { throw new DOMException('Not allowed', 'InvalidStateError'); }
      async delete() {}
      async getAll() { return []; }
    };
  }

  // ContactsManager — CreepJS checks window-level constructor.
  if (typeof window.ContactsManager === 'undefined') {
    window.ContactsManager = class ContactsManager {
      async getProperties() { return ['name', 'email', 'tel']; }
      async select() { throw new DOMException('Not allowed', 'InvalidStateError'); }
    };
  }
})();
