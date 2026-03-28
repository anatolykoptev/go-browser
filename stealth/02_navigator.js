// Navigator property overrides — all values from active stealth profile.
const sp = window.__sp;

if (sp) {
  // webdriver = false (not undefined)
  Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
    get: () => false, configurable: true, enumerable: true
  });

  // userAgentData from profile
  if (!navigator.userAgentData && sp.userAgentData) {
    const uad = sp.userAgentData;
    Object.defineProperty(navigator, 'userAgentData', {
      get: () => ({
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
          fullVersionList: uad.brands.map(b => ({...b})),
        }),
        toJSON: () => ({brands: uad.brands, mobile: uad.mobile, platform: uad.platform}),
      }),
      configurable: true,
    });
  }

  // Hardware from profile
  if (sp.hardware) {
    Object.defineProperty(Navigator.prototype, 'hardwareConcurrency', {
      get: () => sp.hardware.hardwareConcurrency, configurable: true
    });
    Object.defineProperty(Navigator.prototype, 'deviceMemory', {
      get: () => sp.hardware.deviceMemory, configurable: true
    });
    Object.defineProperty(Navigator.prototype, 'maxTouchPoints', {
      get: () => sp.hardware.maxTouchPoints, configurable: true
    });
  }

  // Languages from profile
  if (sp.languages) {
    Object.defineProperty(Navigator.prototype, 'languages', {
      get: () => Object.freeze([...sp.languages]), configurable: true
    });
    Object.defineProperty(Navigator.prototype, 'language', {
      get: () => sp.languages[0], configurable: true
    });
  }

  // Screen from profile
  if (sp.screen) {
    const s = sp.screen;
    for (const [k, v] of Object.entries(s)) {
      if (k === 'devicePixelRatio') {
        Object.defineProperty(window, 'devicePixelRatio', {get: () => v, configurable: true});
      } else {
        Object.defineProperty(screen, k, {get: () => v, configurable: true});
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

  // mediaDevices stub
  if (!navigator.mediaDevices) {
    Object.defineProperty(navigator, 'mediaDevices', {
      get: () => ({
        enumerateDevices: () => Promise.resolve([
          {deviceId: '', groupId: '', kind: 'audioinput', label: ''},
          {deviceId: '', groupId: '', kind: 'videoinput', label: ''},
          {deviceId: '', groupId: '', kind: 'audiooutput', label: ''},
        ]),
        getUserMedia: () => Promise.reject(new DOMException('Permission denied')),
      }),
      configurable: true,
    });
  }
}
