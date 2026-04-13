package browser

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

//go:embed stealth_complement.js
var complementJS string

// NewStealthPage creates a page with stealth evasions applied.
// stealth_complement.js (684 LOC) is the sole stealth layer — go-rod/stealth
// was removed because its Proxy on Function.prototype.toString triggered
// CreepJS hasToStringProxy detection.
//
// Gap B: CDP Emulation.setTimezoneOverride and setLocaleOverride are applied so
// the browser's JS timezone/locale matches the profile, not the host OS.
//
// Gap C: Target.setAutoAttach is enabled so child iframes and workers inherit
// all EvalOnNewDocument injections applied on the parent page.
func (m *ChromeManager) NewStealthPage(ctx *rod.Browser, profile *StealthProfile) (*rod.Page, error) {
	page, err := ctx.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, fmt.Errorf("chrome: create page: %w", err)
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

	// Gap C note: Target.setAutoAttach was previously used to propagate
	// EvalOnNewDocument to child iframes and workers. However, setAutoAttach
	// with Flatten:true causes Chrome to eagerly create browsing contexts for
	// detached iframes with srcdoc set, exposing a non-null contentWindow —
	// a signal detected by CreepJS hasIframeProxy.
	//
	// We deliberately omit setAutoAttach. Worker injection is handled instead
	// by the window.Worker override in stealth_complement.js (05_worker_injection.js)
	// which bootstraps stealth patches via fetch+prepend for all worker URLs.
	// Iframes inherit the page's EvalOnNewDocument scripts via the browser's
	// normal same-origin inheritance (no CDP plumbing needed for same-origin frames).

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

// applyStealthToExistingPage injects stealth JS into an already-created page (EvalOnNewDocument
// + CDP emulation overrides). Used by ContextPool when a page was created without stealth.
func applyStealthToExistingPage(page *rod.Page, profile *StealthProfile) error {
	if profile != nil {
		if _, err := page.EvalOnNewDocument(profile.InjectJS()); err != nil {
			return fmt.Errorf("chrome: inject profile: %w", err)
		}
	}
	if _, err := page.EvalOnNewDocument(complementJS); err != nil {
		return fmt.Errorf("chrome: eval complement js: %w", err)
	}
	if profile != nil {
		if err := applyEmulationOverrides(page, profile); err != nil {
			return err
		}
		if err := setAcceptLanguage(page, profile.Langs); err != nil {
			return err
		}
	}
	return nil
}

// applyEmulationOverrides sets CDP Emulation timezone, locale, and user-agent
// (with full userAgentMetadata for Sec-CH-UA-* headers) from the profile.
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

	// Gap 5b — setUserAgentOverride with full userAgentMetadata so that
	// Sec-CH-UA-* HTTP headers match the profile platform (e.g. "macOS" not "Linux").
	if profile.UA != "" {
		if err := applyUserAgentOverride(page, profile); err != nil {
			return err
		}
	}

	return nil
}

// applyUserAgentOverride calls Emulation.setUserAgentOverride with the full
// userAgentMetadata struct so Sec-CH-UA-Platform and friends are set correctly.
func applyUserAgentOverride(page *rod.Page, profile *StealthProfile) error {
	uad := profile.UAData

	// Build brand lists using the rod proto types.
	toBrandList := func(brands []Brand) []*proto.EmulationUserAgentBrandVersion {
		out := make([]*proto.EmulationUserAgentBrandVersion, len(brands))
		for i, b := range brands {
			out[i] = &proto.EmulationUserAgentBrandVersion{Brand: b.Brand, Version: b.Version}
		}
		return out
	}

	fvl := uad.FullVersionList
	if len(fvl) == 0 {
		fvl = uad.Brands
	}

	acceptLang := ""
	if len(profile.Langs) > 0 {
		var b strings.Builder
		b.WriteString(profile.Langs[0])
		for i, l := range profile.Langs[1:] {
			q := 0.9 - float64(i)*0.1
			if q < 0.1 {
				q = 0.1
			}
			fmt.Fprintf(&b, ",%s;q=%.1f", l, q)
		}
		acceptLang = b.String()
	}

	override := proto.EmulationSetUserAgentOverride{
		UserAgent:      profile.UA,
		AcceptLanguage: acceptLang,
		Platform:       profile.Platform,
		UserAgentMetadata: &proto.EmulationUserAgentMetadata{
			Brands:          toBrandList(uad.Brands),
			FullVersionList: toBrandList(fvl),
			FullVersion:     uad.FullVersion,
			Platform:        uad.Platform,
			PlatformVersion: uad.PlatformVersion,
			Architecture:    uad.Architecture,
			Model:           uad.Model,
			Mobile:          uad.Mobile,
			Bitness:         uad.Bitness,
			Wow64:           uad.Wow64,
		},
	}
	if err := override.Call(page); err != nil {
		return fmt.Errorf("chrome: set user-agent override: %w", err)
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
