// Screen dimension override + matchMedia alignment.
//
// Problem: stealth profile sets screen.width/height but the actual Xvfb
// display may differ. Fingerprint detectors compare screen.width with
// CSS media queries: matchMedia("(device-width: " + screen.width + "px)")
// and flag mismatches as spoofing ("media_mismatch" detection).
//
// Solution: override Screen.prototype properties to match profile values,
// then patch matchMedia to return consistent results for dimension queries.

(() => {
  const sp = window.__sp;
  const scr = (sp && sp.screen) || null;

  // --- Screen properties (profile-dependent) ---
  if (scr) {
    const screenProps = {
      width:       scr.width,
      height:      scr.height,
      availWidth:  scr.availWidth  || scr.width,
      availHeight: scr.availHeight || scr.height,
      colorDepth:  scr.colorDepth  || 24,
      pixelDepth:  scr.pixelDepth  || 24,
    };

    for (const [prop, val] of Object.entries(screenProps)) {
      Object.defineProperty(Screen.prototype, prop, {
        value: val,
        writable: true,
        configurable: true,
        enumerable: true,
      });
    }

    if (scr.devicePixelRatio) {
      Object.defineProperty(window, 'devicePixelRatio', {
        value: scr.devicePixelRatio,
        writable: true,
        configurable: true,
        enumerable: true,
      });
    }

    Object.defineProperty(window, 'outerWidth', {
      value: scr.width,
      writable: true,
      configurable: true,
      enumerable: true,
    });
    Object.defineProperty(window, 'outerHeight', {
      value: scr.availHeight || (scr.height - 25),
      writable: true,
      configurable: true,
      enumerable: true,
    });
  }

  // --- matchMedia patch (ALWAYS active) ---
  // Uses current screen.width/height (may be spoofed above or by CDP).
  // Defeats: matchMedia("(device-width: "+screen.width+"px)").matches === false
  const _origMatchMedia = window.matchMedia;
  if (typeof _origMatchMedia !== 'function') return;

  // Read spoofed values lazily so we pick up whatever is set.
  function getFeatureValue(feature) {
    switch (feature) {
      case 'device-width':  return screen.width;
      case 'device-height': return screen.height;
      case 'width':         return screen.availWidth  || screen.width;
      case 'height':        return screen.availHeight || screen.height;
      default:              return undefined;
    }
  }

  // Parse and evaluate a single media condition against spoofed values.
  // Handles: (feature: Xpx), (min-feature: Xpx), (max-feature: Xpx).
  const dimPattern = /\(\s*(min-|max-)?(device-width|device-height|width|height)\s*:\s*([\d.]+)\s*px\s*\)/g;

  function evaluateQuery(query) {
    let modified = false;
    const result = query.replace(dimPattern, (match, prefix, feature, valStr) => {
      const spoofed = getFeatureValue(feature);
      if (spoofed === undefined) return match;
      modified = true;
      const val = parseFloat(valStr);
      let passes;
      if (prefix === 'min-') {
        passes = spoofed >= val;
      } else if (prefix === 'max-') {
        passes = spoofed <= val;
      } else {
        passes = spoofed === val;
      }
      // Replace with a tautology or contradiction to force correct result.
      return passes ? '(min-width: 1px)' : '(min-width: 999999px)';
    });
    return { modified, query: result };
  }

  Object.defineProperty(window, 'matchMedia', {
    value: function matchMedia(query) {
      if (typeof query === 'string') {
        const ev = evaluateQuery(query);
        if (ev.modified) {
          return _origMatchMedia.call(window, ev.query);
        }
      }
      return _origMatchMedia.call(window, query);
    },
    writable: true,
    configurable: true,
    enumerable: true,
  });
})();
