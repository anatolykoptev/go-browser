// Package webvitals embeds the Google web-vitals.js library (v5.2.0)
// and provides the JS injection code for collecting Core Web Vitals.
package webvitals

import (
	_ "embed"
	"fmt"
)

//go:embed web-vitals.iife.js
var libraryJS string

// InjectionScript returns JS code to be injected via EvalOnNewDocument.
// It loads web-vitals.js and sets up metric collection into window.__cwv.
func InjectionScript() string {
	return fmt.Sprintf(`
%s

window.__cwv = {fcp: -1, lcp: -1, cls: -1, ttfb: -1, inp: -1};
webVitals.onFCP(function(m) { window.__cwv.fcp = m.value; });
webVitals.onLCP(function(m) { window.__cwv.lcp = m.value; });
webVitals.onCLS(function(m) { window.__cwv.cls = m.value; });
webVitals.onTTFB(function(m) { window.__cwv.ttfb = m.value; });
webVitals.onINP(function(m) { window.__cwv.inp = m.value; });
`, libraryJS)
}

// CollectScript returns JS code to retrieve collected metrics.
// Rod's Eval wraps the code in a function, so we just return the value.
func CollectScript() string {
	return `JSON.stringify(window.__cwv || {})`
}
