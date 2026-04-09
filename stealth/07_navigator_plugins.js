// navigator.plugins and navigator.mimeTypes spoofing.
// Chrome 145 always exposes 5 hardcoded PDF plugin entries (since Chrome 92+).
// Profile-driven: reads plugin list from window.__sp.plugins if present,
// otherwise falls back to the canonical 5-entry PDF set.

(() => {
  if (typeof Plugin === 'undefined' || typeof PluginArray === 'undefined') return;

  const DEFAULT_PLUGINS = [
    {name: 'PDF Viewer',                filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
    {name: 'Chrome PDF Viewer',         filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
    {name: 'Chromium PDF Viewer',       filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
    {name: 'Microsoft Edge PDF Viewer', filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
    {name: 'WebKit built-in PDF',       filename: 'internal-pdf-viewer', description: 'Portable Document Format'},
  ];
  const DEFAULT_MIMETYPES = [
    {type: 'application/pdf', suffixes: 'pdf', description: 'Portable Document Format'},
    {type: 'text/pdf',        suffixes: 'pdf', description: 'Portable Document Format'},
  ];

  const sp = window.__sp;
  const pluginData   = (sp && Array.isArray(sp.plugins) && sp.plugins.length > 0)
    ? sp.plugins : DEFAULT_PLUGINS;

  // Build MimeType-like objects with prototype preservation.
  const makeMimeType = (m) => {
    const mt = Object.create(MimeType.prototype);
    Object.defineProperties(mt, {
      type:        {value: m.type,        enumerable: true},
      suffixes:    {value: m.suffixes,    enumerable: true},
      description: {value: m.description, enumerable: true},
      enabledPlugin: {value: null,        enumerable: true},
    });
    return mt;
  };

  const mimeTypes = DEFAULT_MIMETYPES.map(makeMimeType);

  // Build Plugin-like objects. Each plugin exposes its two MIME types by index.
  const makePlugin = (d) => {
    const p = Object.create(Plugin.prototype);
    Object.defineProperties(p, {
      name:        {value: d.name,        enumerable: true},
      filename:    {value: d.filename,    enumerable: true},
      description: {value: d.description, enumerable: true},
      length:      {value: mimeTypes.length},
    });
    mimeTypes.forEach((mt, i) => { p[i] = mt; });
    p.item      = (i) => mimeTypes[i] || null;
    p.namedItem = (n) => mimeTypes.find(m => m.type === n) || null;
    return p;
  };

  const plugins = pluginData.map(makePlugin);

  // Assemble PluginArray.
  const pluginArray = Object.create(PluginArray.prototype);
  plugins.forEach((p, i) => { pluginArray[i] = p; });
  Object.defineProperty(pluginArray, 'length', {value: plugins.length});
  pluginArray.item      = (i) => plugins[i] || null;
  pluginArray.namedItem = (n) => plugins.find(p => p.name === n) || null;
  pluginArray.refresh   = () => {};
  pluginArray[Symbol.iterator] = function* () { yield* plugins; };

  // Assemble MimeTypeArray.
  const mimeTypeArray = Object.create(MimeTypeArray.prototype);
  mimeTypes.forEach((m, i) => { mimeTypeArray[i] = m; });
  Object.defineProperty(mimeTypeArray, 'length', {value: mimeTypes.length});
  mimeTypeArray.item      = (i) => mimeTypes[i] || null;
  mimeTypeArray.namedItem = (n) => mimeTypes.find(m => m.type === n) || null;
  mimeTypeArray[Symbol.iterator] = function* () { yield* mimeTypes; };

  Object.defineProperty(Navigator.prototype, 'plugins', {
    get: () => pluginArray,
    configurable: true,
  });
  Object.defineProperty(Navigator.prototype, 'mimeTypes', {
    get: () => mimeTypeArray,
    configurable: true,
  });
})();
