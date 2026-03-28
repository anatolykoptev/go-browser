// Media codecs, notifications, and permissions overrides.

// Video codec support — headless may report different support.
const origCPT = HTMLMediaElement.prototype.canPlayType;
HTMLMediaElement.prototype.canPlayType = function(type) {
  if (type.includes('h264') || type.includes('avc1')) return 'probably';
  if (type.includes('vp8') || type.includes('vp9')) return 'probably';
  return origCPT.call(this, type);
};

// Notification.permission — headless returns 'denied', real browsers default to 'default'.
if (typeof Notification !== 'undefined') {
  Object.defineProperty(Notification, 'permission', {
    get: () => 'default',
    configurable: true,
  });
}

// Permissions.query — headless returns 'denied' for notifications.
if (typeof Permissions !== 'undefined') {
  const origQuery = Permissions.prototype.query;
  Permissions.prototype.query = function(desc) {
    if (desc.name === 'notifications') {
      return Promise.resolve({state: Notification.permission});
    }
    return origQuery.apply(this, arguments);
  };
}
