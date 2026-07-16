package browser

import (
	"errors"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

// TestIsAlreadyInEffectErr covers the error-classification helper that
// applyEmulationOverrides uses to decide whether a setLocaleOverride /
// setTimezoneOverride failure is the Chrome singleton-controller "another
// override is already in effect" error (non-fatal — the override is already
// set by another page with the same profile) or a real error (fatal).
func TestIsAlreadyInEffectErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"locale already in effect", errors.New(`{-32000 Another locale override is already in effect }`), true},
		{"timezone already in effect", errors.New(`{-32000 Timezone override is already in effect }`), true},
		{"invalid locale name", errors.New("Invalid locale name"), false},
		{"unrelated CDP error", errors.New(`{-32602 Invalid InterceptionId. }`), false},
		{"wrapped already in effect", errors.New("chrome: set locale override: {-32000 Another locale override is already in effect }"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAlreadyInEffectErr(tt.err)
			if got != tt.want {
				t.Errorf("isAlreadyInEffectErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestApplyEmulationOverrides_ParallelLocaleOverride is the integration test
// for the race condition: Chrome's LocaleController and TimeZoneController
// are process-wide singletons, so two pages created concurrently in the same
// browser that both try to set the same locale/timezone override will race —
// the first wins, the second gets "Another locale override is already in
// effect". Before the fix, applyEmulationOverrides returned this as a hard
// error, causing 4 of 5 parallel chrome_interact calls to fail. After the
// fix, the "already in effect" error is logged and skipped (the override is
// already the correct value — all pages use the same profile), so both pages
// succeed.
func TestApplyEmulationOverrides_ParallelLocaleOverride(t *testing.T) {
	b := acquireSharedBrowser(t)

	profile, err := LoadProfile("mac_chrome145")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	m := &ChromeManager{browser: b}
	_, err = m.DefaultContext()
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}

	// Create two pages concurrently and apply stealth to both — the second
	// one will hit the "already in effect" error from the singleton
	// LocaleController/TimeZoneController. The fix makes this non-fatal.
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			page, perr := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
			if perr != nil {
				errs <- perr
				return
			}
			defer func() { _ = page.Close() }()
			errs <- applyStealthToExistingPage(page, profile)
		}()
	}

	for i := 0; i < 2; i++ {
		if e := <-errs; e != nil {
			t.Errorf("applyStealthToExistingPage failed under parallel load: %v", e)
		}
	}
}
