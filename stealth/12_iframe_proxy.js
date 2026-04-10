// iframe.contentWindow proxy fix.
//
// The hasIframeProxy detection checks: document.createElement('iframe') with
// srcdoc set on a detached iframe — contentWindow should be null for real Chrome.
//
// Root cause: Target.setAutoAttach (CDP) was previously enabled and caused
// Chrome's DevTools to eagerly create browsing contexts for iframes even when
// detached, exposing a non-null contentWindow.
//
// Fix: Target.setAutoAttach is NOT called in stealth_page.go (removed in v0.6.20).
// Without it, Chrome behaves like a non-headless browser — contentWindow is null
// for detached iframes. No JS-level interception is needed.
//
// This file is kept as a placeholder/documentation only.
// Worker injection is handled by 05_worker_injection.js (window.Worker override).
