// iframe.contentWindow proxy fix — Target.setAutoAttach (Gap C) enables
// the DevTools protocol to eagerly create browsing contexts for iframes
// with srcdoc set, even when they are not yet appended to the DOM.
// Chrome's C++ binding sets contentWindow directly, bypassing JS Proxy
// traps entirely — so Proxy-based approaches cannot intercept the read.
//
// Root cause: when srcdoc is set on a detached iframe, Chrome immediately
// creates a browsing context and exposes contentWindow. Real Chrome
// (without setAutoAttach) only does this when the iframe is in the DOM.
//
// Fix: intercept the srcdoc setter on HTMLIFrameElement.prototype.
// When srcdoc is set on a detached iframe, defer the actual assignment
// until the iframe is connected. Track the pending value and apply it
// on the first DOM insertion via MutationObserver on parentNode, or
// by overriding insertBefore/appendChild on the element's future parent.
//
// This prevents Chrome from seeing srcdoc on a detached iframe,
// so no browsing context is created, so contentWindow stays null.
// CreepJS hasIframeProxy check: !!iframe.contentWindow → false → pass.

(() => {
  const origSrcdocDesc = Object.getOwnPropertyDescriptor(HTMLIFrameElement.prototype, 'srcdoc');
  if (!origSrcdocDesc) return;

  // Map from iframe element → pending srcdoc value (set while detached).
  const pendingSrcdoc = new WeakMap();

  // Apply any pending srcdoc value once the element is connected.
  function flushPendingSrcdoc(el) {
    if (!pendingSrcdoc.has(el)) return;
    const val = pendingSrcdoc.get(el);
    pendingSrcdoc.delete(el);
    origSrcdocDesc.set.call(el, val);
  }

  Object.defineProperty(HTMLIFrameElement.prototype, 'srcdoc', {
    get() {
      // Return the pending value if not yet flushed, otherwise the real value.
      return pendingSrcdoc.has(this)
        ? pendingSrcdoc.get(this)
        : origSrcdocDesc.get.call(this);
    },
    set(val) {
      if (!this.isConnected) {
        // Defer: store the value but don't tell Chrome yet.
        pendingSrcdoc.set(this, val);
        // Watch for insertion into the DOM.
        const observer = new MutationObserver(() => {
          if (this.isConnected) {
            observer.disconnect();
            flushPendingSrcdoc(this);
          }
        });
        // Observe the document body (or documentElement as fallback).
        const root = document.body || document.documentElement;
        if (root) observer.observe(root, { childList: true, subtree: true });
      } else {
        // Already connected — forward immediately.
        origSrcdocDesc.set.call(this, val);
      }
    },
    configurable: true,
    enumerable: true,
  });
})();
