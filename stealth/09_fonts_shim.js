// Font fingerprint shim — hides residual Linux-only fonts from detection.
// CreepJS probes fonts via document.fonts.check() and forEach().
// We patch document.fonts directly (FontFaceSet is not a global in Chrome 146+).
// This shim also handles final cleanup of window.__sp and stealth markers
// because it runs last (alphabetically 09 > all others).

(() => {
  const HIDDEN_LINUX_FONTS = new Set([
    'Arimo', 'Chilanka', 'Cousine', 'Jomolhari',
    'Liberation Mono', 'Liberation Sans', 'Liberation Serif',
    'Ubuntu', 'Ubuntu Mono', 'Ubuntu Condensed',
    'DejaVu Sans', 'DejaVu Sans Mono', 'DejaVu Serif',
    'Noto Color Emoji', 'MONO',
  ]);

  const matchesHidden = (fontStr) => {
    for (const name of HIDDEN_LINUX_FONTS) {
      if (fontStr.includes('"' + name + '"') || fontStr.includes("'" + name + "'")) {
        return true;
      }
    }
    return false;
  };

  // Patch document.fonts.check() directly — FontFaceSet is not a global in Chrome 146+.
  if (document.fonts && typeof document.fonts.check === 'function') {
    const origCheck = document.fonts.check.bind(document.fonts);
    document.fonts.check = function(font, text) {
      if (matchesHidden(String(font))) return false;
      return origCheck(font, text);
    };
  }

  // Patch document.fonts.forEach() to skip hidden fonts.
  if (document.fonts && typeof document.fonts.forEach === 'function') {
    const origForEach = document.fonts.forEach.bind(document.fonts);
    document.fonts.forEach = function(cb, thisArg) {
      return origForEach(function(ff, k, set) {
        if (HIDDEN_LINUX_FONTS.has(ff.family)) return;
        return cb.call(thisArg, ff, k, set);
      });
    };
  }
})();

// Cleanup — runs last so all prior modules (06-08) can still read __sp.
delete window.__stealthProfile;
delete window.__sp;
delete window.__defineNativeGetter;
