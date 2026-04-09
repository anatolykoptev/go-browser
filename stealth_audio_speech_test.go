package browser

import (
	"strings"
	"testing"
)

// TestStealth_SpeechVoices verifies that speechSynthesis.getVoices() returns
// 34 voices for the macOS profile and that "Samantha" is the default voice.
func TestStealth_SpeechVoices(t *testing.T) {
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

	// SpeechSynthesis may be unavailable on about:blank; navigate to HTML page.
	if err := page.Navigate("data:text/html,<html><body></body></html>"); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	_ = page.WaitLoad()

	res, err := page.Eval(`() => {
		var voices = speechSynthesis.getVoices();
		var samantha = voices.find(function(v){ return v.name === 'Samantha'; });
		return voices.length + '|' + (samantha ? samantha.default : 'MISSING');
	}`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	title := res.Value.Str()

	parts := strings.SplitN(title, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected result: %q", title)
	}
	if parts[0] != "34" {
		t.Errorf("speechSynthesis.getVoices().length = %q, want 34", parts[0])
	}
	if parts[1] != "true" {
		t.Errorf("Samantha default = %q, want true", parts[1])
	}
	t.Logf("speech voices verified: count=%s samantha_default=%s", parts[0], parts[1])
}

// TestStealth_AudioFingerprintStable verifies that OfflineAudioContext renders
// produce a stable (seeded) hash across two consecutive calls in the same profile.
// This confirms CloakBrowser's C++ AudioContext noise seeding is active.
// On local Chromium (not CloakBrowser), the result may vary — the test skips on
// ERROR to allow both environments.
func TestStealth_AudioFingerprintStable(t *testing.T) {
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

	renderAudioHash := func() string {
		page, err := m.NewStealthPage(ctx, profile)
		if err != nil {
			t.Fatalf("NewStealthPage: %v", err)
		}
		defer func() { _ = page.Close() }()

		if err := page.Navigate("about:blank"); err != nil {
			t.Fatalf("navigate: %v", err)
		}
		_ = page.WaitLoad()

		res, err := page.Eval(`() => new Promise(resolve => {
			try {
				var ac = new OfflineAudioContext(1, 44100, 44100);
				var osc = ac.createOscillator();
				var comp = ac.createDynamicsCompressor();
				osc.connect(comp); comp.connect(ac.destination);
				osc.start(0);
				ac.startRendering().then(function(buf) {
					var data = buf.getChannelData(0).slice(4500, 5000);
					var sum = 0;
					for (var i = 0; i < data.length; i++) sum += Math.abs(data[i]);
					resolve(sum.toFixed(6));
				}).catch(function(e) { resolve('ERROR:' + e.message); });
			} catch(e) { resolve('ERROR:' + e.message); }
		})`)
		if err != nil {
			return "ERROR:" + err.Error()
		}
		return res.Value.Str()
	}

	hash1 := renderAudioHash()
	hash2 := renderAudioHash()

	if strings.HasPrefix(hash1, "ERROR") {
		t.Skipf("OfflineAudioContext unavailable: %s", hash1)
	}
	if hash1 != hash2 {
		t.Errorf("audio fingerprint unstable: %q vs %q (seeded noise not working)", hash1, hash2)
	}
	t.Logf("audio fingerprint stable: %q", hash1)
}
