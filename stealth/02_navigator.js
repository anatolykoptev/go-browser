// Native toString masking — make overridden getters look like [native code]
//
// We use a Proxy on Function.prototype.toString rather than replacing it with
// a regular function.  A Proxy around the native toString:
//   - has no .prototype (forwarded to native, which has none)
//   - throws TypeError for new Proxy() (native toString is not a constructor)
//   - passes all descriptor checks (get/set/enumerable come from native)
//   - returns native-looking strings for our spoofed functions via apply trap
//
// This avoids the `lieProps['Function.toString']` detection in CreepJS which
// fires when Function.prototype.toString is replaced with a regular function.
(function() {
  const _nativeToString = Function.prototype.toString;
  const _nativeMap = new WeakMap();

  const _toStringProxy = new Proxy(_nativeToString, {
    apply(target, thisArg, args) {
      const native = _nativeMap.get(thisArg);
      if (native) return native;
      return Reflect.apply(target, thisArg, args);
    },
  });

  // Replace on the prototype — the Proxy wraps the native fn, so all structural
  // checks (typeof, prototype presence, descriptor) forward to the native.
  Object.defineProperty(Function.prototype, 'toString', {
    value: _toStringProxy,
    writable: true,
    configurable: true,
    enumerable: false,
  });

  // The proxy itself must also look native when probed.
  _nativeMap.set(_toStringProxy, 'function toString() { [native code] }');

  // Helper: define a property with a getter that reports as native code
  window.__defineNativeGetter = function(obj, prop, getter, nativeName) {
    _nativeMap.set(getter, 'function get ' + (nativeName || prop) + '() { [native code] }');
    Object.defineProperty(obj, prop, {
      get: getter,
      configurable: true,
      enumerable: true
    });
  };
})();

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
