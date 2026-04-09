// WebRTC local IP leak prevention.
// Wraps RTCPeerConnection (and legacy webkitRTCPeerConnection) to:
//   1. Strip STUN servers from ICE config — no mDNS/RFC1918 gathering.
//   2. Filter .local mDNS and RFC1918 candidates from icecandidate events.
// Preserves prototype chain and masks toString to pass native-code checks.

(() => {
  const RFC1918 = /\b(10\.\d+\.\d+\.\d+|127\.\d+\.\d+\.\d+|192\.168\.\d+\.\d+|172\.(1[6-9]|2\d|3[01])\.\d+\.\d+)\b/;
  const MDNS    = /\.local\b/;
  const STUN    = /^stun:/i;

  const isPrivateCandidate = (candidateStr) =>
    MDNS.test(candidateStr) || RFC1918.test(candidateStr);

  const wrap = (OrigPC) => {
    if (typeof OrigPC !== 'function') return OrigPC;

    const Wrapped = function RTCPeerConnection(config, ...rest) {
      if (config && Array.isArray(config.iceServers)) {
        config = Object.assign({}, config, {
          iceServers: config.iceServers.filter(s => {
            const urls = [].concat(s.urls || s.url || []);
            return urls.every(u => !STUN.test(u));
          }),
        });
      }

      const pc = new OrigPC(config, ...rest);

      // Intercept addEventListener to filter icecandidate events.
      const origAdd = pc.addEventListener.bind(pc);
      pc.addEventListener = function(type, cb, ...opts) {
        if (type !== 'icecandidate' || typeof cb !== 'function') {
          return origAdd(type, cb, ...opts);
        }
        return origAdd(type, (ev) => {
          if (ev.candidate && ev.candidate.candidate &&
              isPrivateCandidate(ev.candidate.candidate)) {
            return; // drop private candidate
          }
          cb(ev);
        }, ...opts);
      };

      // Mirror onicecandidate setter through the filtered addEventListener.
      Object.defineProperty(pc, 'onicecandidate', {
        set(fn) { pc.addEventListener('icecandidate', fn); },
        get() { return null; },
        configurable: true,
      });

      return pc;
    };

    // Preserve prototype identity so instanceof checks pass.
    Wrapped.prototype = OrigPC.prototype;
    Object.setPrototypeOf(Wrapped, OrigPC);
    Wrapped.toString = () => OrigPC.toString();

    return Wrapped;
  };

  if (typeof window.RTCPeerConnection !== 'undefined') {
    window.RTCPeerConnection = wrap(window.RTCPeerConnection);
  }
  if (typeof window.webkitRTCPeerConnection !== 'undefined') {
    window.webkitRTCPeerConnection = wrap(window.webkitRTCPeerConnection);
  }
})();
