// Navigator property overrides for headless detection evasion.

// webdriver must be false (not undefined).
// Chrome with --disable-blink-features=AutomationControlled returns false.
Object.defineProperty(Object.getPrototypeOf(navigator), 'webdriver', {
  get: () => false, configurable: true, enumerable: true
});

// NavigatorUAData (Chrome Client Hints).
// Headless Chrome lacks navigator.userAgentData — critical for Castle.io.
// Platform must match CloakBrowser's --fingerprint-platform and GPU renderer.
// CloakBrowser with SwiftShader reports "Intel Iris OpenGL Engine" = macOS GPU.
if (!navigator.userAgentData) {
  const brands = [
    {brand: 'Chromium', version: '145'},
    {brand: 'Google Chrome', version: '145'},
    {brand: 'Not-A.Brand', version: '24'}
  ];
  Object.defineProperty(navigator, 'userAgentData', {
    get: () => ({
      brands: brands,
      mobile: false,
      platform: 'macOS',
      getHighEntropyValues: (hints) => Promise.resolve({
        brands: brands,
        mobile: false,
        platform: 'macOS',
        platformVersion: '14.5.0',
        architecture: 'arm',
        bitness: '64',
        model: '',
        uaFullVersion: '145.0.7632.159',
        fullVersionList: brands.map(b => ({...b})),
      }),
      toJSON: () => ({brands: brands, mobile: false, platform: 'macOS'}),
    }),
    configurable: true,
  });
}

// mediaDevices stub — headless Chrome lacks media devices.
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
