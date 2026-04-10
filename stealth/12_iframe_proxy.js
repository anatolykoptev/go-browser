// iframe.contentWindow proxy fix — Target.setAutoAttach (Gap C) enables
// the DevTools protocol to eagerly create browsing contexts for iframes
// with srcdoc set, even when they are not yet appended to the DOM.
// Chrome eagerly sets contentWindow as an OWN property on the iframe
// instance. Real Chrome returns null contentWindow for detached iframes.
// CreepJS hasIframeProxy detects this discrepancy.
//
// Fix: intercept iframe creation via document.createElement override.
// For each new iframe, install a defineProperty trap on the element
// that, when Chrome sets contentWindow as an own property, wraps it
// in a getter that returns null when the iframe is not connected.

(() => {
  const origCreateElement = Document.prototype.createElement;

  Document.prototype.createElement = function(tag, options) {
    const el = origCreateElement.call(this, tag, options);
    if (typeof tag === 'string' && tag.toLowerCase() === 'iframe') {
      // Install a defineProperty trap on this specific iframe element.
      // Chrome (via Target.setAutoAttach) will try to set contentWindow
      // as an own data property. We intercept that set and replace it
      // with a conditional getter.
      const origDefineProperty = Object.defineProperty;
      const patchContentWindow = (element) => {
        let realWindow = null;

        // Wait for Chrome to set the own contentWindow property.
        const observer = new MutationObserver(() => {});
        // Use a one-shot defineProperty trap on the element:
        const origProto = Object.getPrototypeOf(element);
        origDefineProperty.call(Object, element, 'contentWindow', {
          get() {
            return element.isConnected ? realWindow : null;
          },
          set(v) {
            realWindow = v;
          },
          configurable: true,
          enumerable: true,
        });
      };
      patchContentWindow(el);
    }
    return el;
  };
})();
