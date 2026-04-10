// Storage estimate spoof — return macOS-realistic quota/usage values.
//
// Headless Chrome returns a tiny quota (container disk space) which
// fingerprint detectors use to identify non-desktop environments.
// Real macOS with 512GB SSD typically reports ~450-500GB quota with
// 30-40% usage depending on the machine state.
//
// Spec numbers for mac_chrome145 profile:
//   quota:  494384795648  (~460 GB — typical 512GB Mac after OS overhead)
//   usage:  189654345216  (~176 GB — ~38% used, realistic for a work machine)

(() => {
  if (!navigator.storage || typeof navigator.storage.estimate !== 'function') return;

  const MAC_QUOTA = 494384795648;
  const MAC_USAGE = 189654345216;

  const _origEstimate = navigator.storage.estimate.bind(navigator.storage);

  // Use Object.defineProperty so the replacement isn't enumerable and
  // doesn't add an own .prototype that lieProps['StorageManager.estimate'] would flag.
  Object.defineProperty(navigator.storage, 'estimate', {
    value: () => Promise.resolve({
      quota: MAC_QUOTA,
      usage: MAC_USAGE,
      usageDetails: {},
    }),
    writable: true,
    configurable: true,
    enumerable: true,
  });
})();
