// iframe.contentWindow proxy fix — Target.setAutoAttach (Gap C) enables
// the DevTools protocol to eagerly create browsing contexts for iframes
// with srcdoc set, even when they are not yet appended to the DOM.
// Real Chrome returns null for contentWindow of detached iframes.
// CreepJS hasIframeProxy detects this discrepancy.
//
// Fix: override HTMLIFrameElement.prototype.contentWindow getter to return
// null when the iframe is not connected to the document.

(() => {
  const origDescriptor = Object.getOwnPropertyDescriptor(
    HTMLIFrameElement.prototype, 'contentWindow'
  );
  if (!origDescriptor || !origDescriptor.get) return;

  const origGetter = origDescriptor.get;

  Object.defineProperty(HTMLIFrameElement.prototype, 'contentWindow', {
    get() {
      // isConnected checks if the element is attached to the document tree.
      // Detached iframes must return null to match real Chrome behavior.
      if (!this.isConnected) return null;
      return origGetter.call(this);
    },
    configurable: true,
    enumerable: true,
  });
})();
