package browser

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/ysmood/gson"
)

//go:embed stealth_complement.js
var complementJS string

// NewStealthPage creates a page with stealth evasions applied.
// It runs go-rod/stealth JS patches followed by the complement JS that fills gaps
// not covered by CloakBrowser's C++ patches.
//
// Gap B: CDP Emulation.setTimezoneOverride and setLocaleOverride are applied so
// the browser's JS timezone/locale matches the profile, not the host OS.
//
// Gap C: Target.setAutoAttach is enabled so child iframes and workers inherit
// all EvalOnNewDocument injections applied on the parent page.
func (m *ChromeManager) NewStealthPage(ctx *rod.Browser, profile *StealthProfile) (*rod.Page, error) {
	page, err := stealth.Page(ctx)
	if err != nil {
		return nil, fmt.Errorf("chrome: stealth page: %w", err)
	}

	// Inject profile data before complement JS so modules can read __sp.
	if profile != nil {
		if _, err := page.EvalOnNewDocument(profile.InjectJS()); err != nil {
			_ = page.Close()
			return nil, fmt.Errorf("chrome: inject profile: %w", err)
		}
	}

	if _, err := page.EvalOnNewDocument(complementJS); err != nil {
		_ = page.Close()
		return nil, fmt.Errorf("chrome: eval complement js: %w", err)
	}

	// Gap B — Timezone & Locale: apply CDP overrides so JS Intl APIs and
	// navigator.language match the profile, not the host OS.
	if profile != nil {
		if err := applyEmulationOverrides(page, profile); err != nil {
			_ = page.Close()
			return nil, err
		}
	}

	// Gap C — Target.setAutoAttach: child iframes and workers inherit the
	// EvalOnNewDocument injections applied above.
	autoAttach := proto.TargetSetAutoAttach{
		AutoAttach:             true,
		WaitForDebuggerOnStart: false,
		Flatten:                true,
	}
	if err := autoAttach.Call(page); err != nil {
		_ = page.Close()
		return nil, fmt.Errorf("chrome: set auto attach: %w", err)
	}

	// Set Accept-Language header to match profile languages so HTTP headers
	// and navigator.languages are consistent (detection scripts compare both).
	if profile != nil && len(profile.Langs) > 0 {
		if err := setAcceptLanguage(page, profile.Langs); err != nil {
			_ = page.Close()
			return nil, err
		}
	}

	return page, nil
}

// applyEmulationOverrides sets CDP Emulation timezone and locale from the profile.
func applyEmulationOverrides(page *rod.Page, profile *StealthProfile) error {
	if profile.Timezone != "" {
		tzOverride := proto.EmulationSetTimezoneOverride{TimezoneID: profile.Timezone}
		if err := tzOverride.Call(page); err != nil {
			return fmt.Errorf("chrome: set timezone override: %w", err)
		}
	}

	if len(profile.Langs) > 0 {
		// CDP locale uses ICU format (e.g. "en-US"); first language tag is the primary.
		localeOverride := proto.EmulationSetLocaleOverride{Locale: profile.Langs[0]}
		if err := localeOverride.Call(page); err != nil {
			return fmt.Errorf("chrome: set locale override: %w", err)
		}
	}

	return nil
}

// setAcceptLanguage sets the Accept-Language HTTP header from ordered language tags.
// Builds a quality-weighted value: "en-US,en;q=0.9,fr;q=0.8".
func setAcceptLanguage(page *rod.Page, langs []string) error {
	if len(langs) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString(langs[0])
	for i, l := range langs[1:] {
		q := 0.9 - float64(i)*0.1
		if q < 0.1 {
			q = 0.1
		}
		fmt.Fprintf(&b, ",%s;q=%.1f", l, q)
	}

	_ = proto.NetworkSetExtraHTTPHeaders{
		Headers: proto.NetworkHeaders{"Accept-Language": gson.New(b.String())},
	}.Call(page)

	return nil
}
