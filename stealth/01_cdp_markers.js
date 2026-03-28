// CDP automation marker cleanup.
// Removes rod/ChromeDriver/Playwright markers from window
// and watches for dynamically injected marker attributes.

const markerPatterns = [/^\$cdc_/, /^\$chrome_/, /^__webdriver/, /^__selenium/, /^__playwright/, /^__pw_/];
for (const key of Object.keys(window)) {
  if (markerPatterns.some(p => p.test(key))) {
    try { delete window[key]; } catch(e) {}
  }
}
try { delete window.__cdp_runtime; } catch(e) {}

const obs = new MutationObserver(mutations => {
  for (const m of mutations) {
    if (m.type === 'attributes' && markerPatterns.some(p => p.test(m.attributeName))) {
      document.documentElement.removeAttribute(m.attributeName);
    }
  }
});
if (document.documentElement) {
  obs.observe(document.documentElement, { attributes: true });
}

// Prevent stack-based CDP detection via Error.prepareStackTrace setter.
const origPST = Error.prepareStackTrace;
Object.defineProperty(Error, 'prepareStackTrace', {
  get: () => origPST,
  set: () => {},
  configurable: false,
});
