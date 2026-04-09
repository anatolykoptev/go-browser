// Font fingerprint shim — hides residual Linux-only fonts from detection.
// CreepJS probes fonts via FontFaceSet.check() and FontFaceSet.forEach().
// We intercept both to return false/skip for known Linux-exclusive fonts.
// macOS-specific fonts (from profile.fonts) are not present on Linux Docker,
// so the Dockerfile.cloakbrowser layer installs them; this shim only hides
// Linux fonts that would betray the host OS when spoofing macOS.

(() => {
  const HIDDEN_LINUX_FONTS = new Set([
    'Arimo', 'Chilanka', 'Cousine', 'Jomolhari',
    'Liberation Mono', 'Liberation Sans', 'Liberation Serif',
    'Ubuntu', 'Ubuntu Mono', 'Ubuntu Condensed',
    'DejaVu Sans', 'DejaVu Sans Mono', 'DejaVu Serif',
    'Noto Color Emoji', 'MONO',
  ]);

  // FontFaceSet.check() — returns false for hidden fonts
  const origCheck = FontFaceSet.prototype.check;
  FontFaceSet.prototype.check = function(font, text) {
    for (const name of HIDDEN_LINUX_FONTS) {
      if (font.includes('"' + name + '"') || font.includes("'" + name + "'")) {
        return false;
      }
    }
    return origCheck.call(this, font, text);
  };

  // FontFaceSet.forEach() — skip hidden font entries
  const origForEach = FontFaceSet.prototype.forEach;
  FontFaceSet.prototype.forEach = function(cb, thisArg) {
    return origForEach.call(this, function(ff, k, set) {
      if (HIDDEN_LINUX_FONTS.has(ff.family)) return;
      return cb.call(thisArg, ff, k, set);
    });
  };

  // FontFaceSet[Symbol.iterator] — skip hidden font entries
  const origValues = FontFaceSet.prototype[Symbol.iterator];
  if (origValues) {
    FontFaceSet.prototype[Symbol.iterator] = function* () {
      for (const ff of origValues.call(this)) {
        if (!HIDDEN_LINUX_FONTS.has(ff.family)) yield ff;
      }
    };
  }
})();
