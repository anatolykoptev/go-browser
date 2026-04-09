package browser

import (
	"strings"
	"testing"
)

// TestStealth_SecChUaPlatform verifies that Emulation.setUserAgentOverride
// with userAgentMetadata causes navigator.userAgentData.platform to report "macOS".
func TestStealth_SecChUaPlatform(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	m := &ChromeManager{browser: b}
	ctx, err := m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	page, err := m.NewStealthPage(ctx, profile)
	if err != nil {
		t.Fatalf("NewStealthPage: %v", err)
	}
	defer func() { _ = page.Close() }()

	if err := page.Navigate("about:blank"); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_ = page.WaitLoad()

	res, err := page.Eval(`() => navigator.userAgentData ? navigator.userAgentData.platform : 'NO_UAD'`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	got := res.Value.Str()
	if got != "macOS" {
		t.Errorf("userAgentData.platform = %q, want %q", got, "macOS")
	}
	t.Logf("sec-ch-ua platform verified: %q", got)
}

// TestStealth_WebRTCNoLeak verifies that no .local mDNS or RFC1918 ICE candidates
// are surfaced after the WebRTC wrapper is applied.
func TestStealth_WebRTCNoLeak(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	m := &ChromeManager{browser: b}
	ctx, err := m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	page, err := m.NewStealthPage(ctx, profile)
	if err != nil {
		t.Fatalf("NewStealthPage: %v", err)
	}
	defer func() { _ = page.Close() }()

	if err := page.Navigate("about:blank"); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_ = page.WaitLoad()

	// Gather ICE candidates for 2 s and collect any private ones.
	// Runs as a Promise so it waits for the setTimeout to fire.
	res, err := page.Eval(`() => new Promise(resolve => {
		var leaked = [];
		try {
			var pc = new RTCPeerConnection({iceServers:[{urls:'stun:stun.l.google.com:19302'}]});
			pc.createDataChannel('x');
			pc.onicecandidate = function(ev) {
				if (ev.candidate && ev.candidate.candidate) leaked.push(ev.candidate.candidate);
			};
			pc.createOffer().then(function(o) { return pc.setLocalDescription(o); });
			setTimeout(function() {
				var private = leaked.filter(function(c) {
					return /\.local\b/.test(c) || /\b(10\.\d+\.\d+\.\d+|127\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(c);
				});
				resolve(private.join('|') || 'CLEAN');
			}, 2000);
		} catch(e) { resolve('CLEAN'); }
	})`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	got := res.Value.Str()
	if got != "CLEAN" {
		t.Errorf("private ICE candidates leaked: %s", got)
	}
	t.Logf("WebRTC leak test: %q", got)
}

// TestStealth_NavigatorPlugins verifies that navigator.plugins exposes exactly
// 5 entries and the first entry is named "PDF Viewer".
func TestStealth_NavigatorPlugins(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	m := &ChromeManager{browser: b}
	ctx, err := m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	page, err := m.NewStealthPage(ctx, profile)
	if err != nil {
		t.Fatalf("NewStealthPage: %v", err)
	}
	defer func() { _ = page.Close() }()

	if err := page.Navigate("about:blank"); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_ = page.WaitLoad()

	res, err := page.Eval(`() => navigator.plugins.length + '|' + (navigator.plugins[0] ? navigator.plugins[0].name : 'NONE')`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	title := res.Value.Str()

	parts := strings.SplitN(title, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected result: %q", title)
	}
	if parts[0] != "5" {
		t.Errorf("navigator.plugins.length = %q, want %q", parts[0], "5")
	}
	if parts[1] != "PDF Viewer" {
		t.Errorf("plugins[0].name = %q, want %q", parts[1], "PDF Viewer")
	}
	t.Logf("navigator.plugins verified: length=%s first=%s", parts[0], parts[1])
}

// TestStealth_FontsShimHidesLinux verifies that known Linux-only fonts report
// as unavailable after the fonts shim is applied. Must navigate to an HTML page
// (not about:blank) so FontFaceSet is available when EvalOnNewDocument runs.
func TestStealth_FontsShimHidesLinux(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	m := &ChromeManager{browser: b}
	ctx, err := m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	page, err := m.NewStealthPage(ctx, profile)
	if err != nil {
		t.Fatalf("NewStealthPage: %v", err)
	}
	defer func() { _ = page.Close() }()

	// FontFaceSet is not available on about:blank; navigate to a real HTML doc.
	if err := page.Navigate("data:text/html,<html><body></body></html>"); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_ = page.WaitLoad()

	res, err := page.Eval(`() => document.fonts.check('12px "Ubuntu"') + '|' + document.fonts.check('12px "DejaVu Sans"')`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	title := res.Value.Str()

	parts := strings.SplitN(title, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected result: %q", title)
	}
	if parts[0] != "false" {
		t.Errorf(`fonts.check("Ubuntu") = %q, want "false"`, parts[0])
	}
	if parts[1] != "false" {
		t.Errorf(`fonts.check("DejaVu Sans") = %q, want "false"`, parts[1])
	}
	t.Logf("fonts shim verified: Ubuntu=%s DejaVuSans=%s", parts[0], parts[1])
}
