// Native toString masking — go-rod/stealth already patches Function.prototype.toString
// via its utils.patchToString mechanism. We do NOT add another proxy layer on top
// because double-wrapping breaks CreepJS's chain-cycle TypeError check, causing
// lieProps['Function.toString'] to be set → hasToStringProxy = true (detected).
//
// Instead, we use go-rod/stealth's already-installed toString override, which
// correctly handles all structural checks including the chain cycle test.
//
// For our custom getters, we expose a simple helper that registers native-looking
// toString strings via the existing mechanism without double-wrapping.

// Helper: define a property with a getter. go-rod/stealth's patchToString
// will handle native-string masking for any function that needs it.
// We use a direct Object.defineProperty without touching Function.prototype.toString.
window.__defineNativeGetter = function(obj, prop, getter, nativeName) {
  Object.defineProperty(obj, prop, {
    get: getter,
    configurable: true,
    enumerable: true
  });
};

// Navigator property overrides — all values from active stealth profile.
const sp = window.__sp;

if (sp) {
  // webdriver = false (not undefined)
  window.__defineNativeGetter(Object.getPrototypeOf(navigator), 'webdriver', () => false);

  // userAgentData from profile
  if (!navigator.userAgentData && sp.userAgentData) {
    const uad = sp.userAgentData;
    const fvl = uad.fullVersionList || uad.brands.map(b => ({...b}));
    window.__defineNativeGetter(navigator, 'userAgentData', () => ({
      brands: uad.brands,
      mobile: uad.mobile,
      platform: uad.platform,
      getHighEntropyValues: (hints) => Promise.resolve({
        brands: uad.brands,
        mobile: uad.mobile,
        platform: uad.platform,
        platformVersion: uad.platformVersion,
        architecture: uad.architecture,
        bitness: uad.bitness,
        model: '',
        uaFullVersion: uad.fullVersion,
        fullVersionList: fvl,
      }),
      toJSON: () => ({brands: uad.brands, mobile: uad.mobile, platform: uad.platform}),
    }));
  }

  // Hardware from profile
  if (sp.hardware) {
    window.__defineNativeGetter(Navigator.prototype, 'hardwareConcurrency',
      () => sp.hardware.hardwareConcurrency);
    window.__defineNativeGetter(Navigator.prototype, 'deviceMemory',
      () => sp.hardware.deviceMemory);
    window.__defineNativeGetter(Navigator.prototype, 'maxTouchPoints',
      () => sp.hardware.maxTouchPoints);
  }

  // Languages from profile
  if (sp.languages) {
    window.__defineNativeGetter(Navigator.prototype, 'languages',
      () => Object.freeze([...sp.languages]));
    window.__defineNativeGetter(Navigator.prototype, 'language',
      () => sp.languages[0]);
  }

  // Screen from profile
  if (sp.screen) {
    const s = sp.screen;
    for (const [k, v] of Object.entries(s)) {
      if (k === 'devicePixelRatio') {
        window.__defineNativeGetter(window, 'devicePixelRatio', () => v);
      } else {
        window.__defineNativeGetter(screen, k, () => v);
      }
    }
  }

  // GPU — WebGL vendor/renderer spoofing from profile
  if (sp.gpu) {
    const spoofWebGL = (proto) => {
      const orig = proto.getParameter;
      proto.getParameter = function(param) {
        if (param === 37445) return sp.gpu.vendor;
        if (param === 37446) return sp.gpu.renderer;
        return orig.apply(this, arguments);
      };
    };
    spoofWebGL(WebGLRenderingContext.prototype);
    if (typeof WebGL2RenderingContext !== 'undefined') {
      spoofWebGL(WebGL2RenderingContext.prototype);
    }
  }

  // NetworkInformation API from profile
  if (sp.connection && 'connection' in navigator) {
    const conn = sp.connection;
    const connProxy = {};
    for (const [k, v] of Object.entries(conn)) {
      window.__defineNativeGetter(connProxy, k, () => v);
    }
    connProxy.addEventListener = function() {};
    connProxy.removeEventListener = function() {};
    connProxy.onchange = null;
    window.__defineNativeGetter(navigator, 'connection', () => connProxy);
  }

  // mediaDevices stub
  if (!navigator.mediaDevices) {
    window.__defineNativeGetter(navigator, 'mediaDevices', () => ({
      enumerateDevices: () => Promise.resolve([
        {deviceId: '', groupId: '', kind: 'audioinput', label: ''},
        {deviceId: '', groupId: '', kind: 'videoinput', label: ''},
        {deviceId: '', groupId: '', kind: 'audiooutput', label: ''},
      ]),
      getUserMedia: () => Promise.reject(new DOMException('Permission denied')),
    }));
  }

  // document.hasFocus — headless returns false, real browser returns true
  document.hasFocus = function() { return true; };
  document.hasFocus.toString = function() { return 'function hasFocus() { [native code] }'; };

  // outerWidth/outerHeight — headless returns 0, real browser matches window
  if (sp.screen) {
    Object.defineProperty(window, 'outerWidth', {
      get: () => sp.screen.width, configurable: true
    });
    Object.defineProperty(window, 'outerHeight', {
      get: () => sp.screen.height + 77, configurable: true // 77px = title+toolbar on macOS
    });
    Object.defineProperty(window, 'screenX', {get: () => 0, configurable: true});
    Object.defineProperty(window, 'screenY', {get: () => 25, configurable: true}); // below menu bar
  }

  // navigator.platform from profile
  if (sp.platform) {
    window.__defineNativeGetter(Navigator.prototype, 'platform', () => sp.platform);
  }

  // Battery API fallback for environments where it's not available
  if (typeof navigator.getBattery !== 'function') {
    navigator.getBattery = function() {
      return Promise.resolve({
        charging: true, chargingTime: 0, dischargingTime: Infinity,
        level: 0.87 + Math.random() * 0.1,
        addEventListener: function() {}, removeEventListener: function() {}
      });
    };
  }

  // Gamepad API — real Chrome returns [null, null, null, null]
  if (typeof navigator.getGamepads !== 'function') {
    navigator.getGamepads = function() { return [null, null, null, null]; };
  }
}
