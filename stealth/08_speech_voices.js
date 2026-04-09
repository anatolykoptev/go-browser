// SpeechSynthesis.getVoices() spoofing.
// Linux/Docker Chrome returns [] — a unique headless signal.
// Profile-driven: reads voice list from window.__sp.voices if present.
// Falls back to empty list for non-macOS profiles (realistic for Linux/Windows).

(() => {
  const sp = window.__sp;
  const voiceData = (sp && Array.isArray(sp.voices)) ? sp.voices : [];
  if (voiceData.length === 0) return; // skip patching for profiles without voices

  const voices = voiceData.map(v => Object.freeze({
    default:      v.default === true,
    lang:         v.lang,
    localService: true,
    name:         v.name,
    voiceURI:     v.voiceURI,
  }));

  if (typeof SpeechSynthesis !== 'undefined') {
    SpeechSynthesis.prototype.getVoices = function() { return voices.slice(); };
  }

  // Fire voiceschanged so pages that wait for the event get real voices.
  setTimeout(() => {
    if (typeof window.speechSynthesis !== 'undefined' &&
        typeof window.speechSynthesis.onvoiceschanged === 'function') {
      window.speechSynthesis.onvoiceschanged(new Event('voiceschanged'));
    }
  }, 100);
})();
