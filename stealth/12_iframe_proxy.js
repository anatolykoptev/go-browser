// iframe.contentWindow proxy fix — Target.setAutoAttach (Gap C) enables
// the DevTools protocol to eagerly create browsing contexts for iframes
// with srcdoc set, even when they are not yet appended to the DOM.
// Chrome sets contentWindow via C++ internals as an OWN data property on
// the iframe instance, bypassing JS set traps (Object.defineProperty).
// Real Chrome returns null contentWindow for detached iframes.
// CreepJS hasIframeProxy detects this discrepancy.
//
// Fix: wrap each new iframe in a Proxy. A Proxy get trap intercepts ALL
// reads regardless of how the underlying property was stored (C++ or JS).
// When contentWindow is read on a detached iframe, return null.

(() => {
  const origCreateElement = Document.prototype.createElement;

  Document.prototype.createElement = function(tag, options) {
    const el = origCreateElement.call(this, tag, options);
    if (typeof tag !== 'string' || tag.toLowerCase() !== 'iframe') {
      return el;
    }

    const proxy = new Proxy(el, {
      get(target, prop, receiver) {
        if (prop === 'contentWindow') {
          // Read the actual value Chrome stored (may be own C++ property).
          const val = target.contentWindow;
          // Return null when the iframe is not in the document — matches
          // real Chrome behaviour for detached iframes without setAutoAttach.
          return target.isConnected ? val : null;
        }
        const val = Reflect.get(target, prop, receiver);
        // Bind functions so `this` inside them refers to the real element.
        if (typeof val === 'function') {
          return val.bind(target);
        }
        return val;
      },
      set(target, prop, value, receiver) {
        return Reflect.set(target, prop, value, target);
      },
    });

    return proxy;
  };
})();
