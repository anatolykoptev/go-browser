// Font fingerprint shim — accurate font detection for headless Chrome.
//
// Problem: headless Chrome's document.fonts.check() returns true for ALL fonts
// (including random nonexistent names), and FontFace.load('local(...)') fails
// with "network error" for every font. Both lie to CreepJS font detection.
//
// Fix: replace document.fonts.check() with a set-based implementation that
// accurately returns true only for fonts that match the installed set
// (Apple system fonts + common cross-platform fonts) and false for everything
// else — including Linux-exclusive fonts that betray the host OS.
//
// This shim also handles final cleanup of window.__sp and stealth markers
// because it runs last (alphabetically 09 > all others).

(() => {
  // Linux-only fonts — must return false when spoofing macOS.
  const LINUX_FONTS = new Set([
    'Arimo', 'Chilanka', 'Cousine', 'Jomolhari',
    'Liberation Mono', 'Liberation Sans', 'Liberation Serif',
    'Ubuntu', 'Ubuntu Mono', 'Ubuntu Condensed',
    'DejaVu Sans', 'DejaVu Sans Mono', 'DejaVu Serif',
    'Noto Color Emoji', 'MONO',
  ]);

  // Common macOS / cross-platform fonts that headless Chrome has via our
  // Dockerfile Apple font layer + base Chrome font packages.
  // These are the fonts CreepJS probes from its MacOSFonts + common sets.
  const MAC_SYSTEM_FONTS = new Set([
    // Core system fonts (pre-installed in Chrome base image)
    'Arial', 'Arial Black', 'Arial Narrow', 'Arial Unicode MS',
    'Comic Sans MS', 'Courier New', 'Georgia', 'Impact',
    'Times New Roman', 'Trebuchet MS', 'Verdana', 'Webdings',
    // Apple macOS system fonts (installed via Dockerfile Apple font layer)
    'SF Pro', 'SF Pro Display', 'SF Pro Text', 'SF Pro Rounded',
    'SF Compact', 'SF Compact Display', 'SF Compact Text',
    'SF Mono', 'New York',
    'Helvetica', 'Helvetica Neue',
    'Apple SD Gothic Neo', 'Apple SD Gothic Neo ExtraBold',
    'Geneva',
    // macOS version-specific fonts (from CreepJS MacOSFonts)
    'Kohinoor Devanagari Medium', 'Luminari',
    'PingFang HK Light',
    'American Typewriter Semibold', 'Futura Bold',
    'SignPainter-HouseScript Semibold',
    'InaiMathi Bold',
    'Galvji', 'MuktaMahee Regular',
    'Noto Sans Gunjala Gondi Regular', 'Noto Sans Masaram Gondi Regular',
    'Noto Serif Yezidi Regular',
    'STIX Two Math Regular', 'STIX Two Text Regular',
    'Noto Sans Canadian Aboriginal Regular',
  ]);

  // Merge profile-specific fonts if available.
  const sp = window.__sp;
  const profileFonts = (sp && sp.fonts) ? sp.fonts : [];

  // Build the full allow-set.
  const ALLOWED = new Set([...MAC_SYSTEM_FONTS]);
  for (const f of profileFonts) ALLOWED.add(f);

  // Extract the font family name from a CSS font shorthand string.
  // e.g. '16px "SF Pro Display"' → 'SF Pro Display'
  //      "0px 'Helvetica Neue'"  → 'Helvetica Neue'
  //      '12px Arial'            → 'Arial'
  const extractFamily = (fontStr) => {
    const s = String(fontStr);
    // Quoted name: extract between quotes
    const quoted = s.match(/["']([^"']+)["']/);
    if (quoted) return quoted[1];
    // Unquoted: last token(s) after the size/style info
    const parts = s.trim().split(/\s+/);
    // Font shorthand: size is last numeric token; family follows.
    // Simple heuristic: return everything after last size-like token.
    const sizeIdx = parts.findIndex(p => /^\d/.test(p));
    if (sizeIdx >= 0 && sizeIdx + 1 < parts.length) {
      return parts.slice(sizeIdx + 1).join(' ');
    }
    return parts[parts.length - 1];
  };

  // Replace document.fonts.check with accurate implementation.
  if (document.fonts && typeof document.fonts.check === 'function') {
    document.fonts.check = function(font, text) {
      const family = extractFamily(font);
      if (LINUX_FONTS.has(family)) return false;
      if (ALLOWED.has(family)) return true;
      // Unknown font: return false (no random-font-returns-true headless bug).
      return false;
    };
  }

  // Replace document.fonts.forEach to skip hidden Linux fonts.
  if (document.fonts && typeof document.fonts.forEach === 'function') {
    const origForEach = document.fonts.forEach.bind(document.fonts);
    document.fonts.forEach = function(cb, thisArg) {
      return origForEach(function(ff, k, set) {
        if (LINUX_FONTS.has(ff.family)) return;
        return cb.call(thisArg, ff, k, set);
      });
    };
  }
})();

// Cleanup — runs last so all prior modules (06-08) can still read __sp.
delete window.__stealthProfile;
delete window.__sp;
delete window.__defineNativeGetter;
