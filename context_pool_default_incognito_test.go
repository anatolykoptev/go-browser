package browser

import (
	"os"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// launchDedicatedBrowser starts a fresh, isolated headless Chromium that this
// test owns exclusively. The stale-default suite shares one browser whose target
// list accumulates pages across tests (ContextPool.Close does not close pages),
// so it cannot offer a deterministic "the incognito page is the only page target"
// state. A dedicated instance gives full control of the target set.
func launchDedicatedBrowser(t *testing.T) (*rod.Browser, func()) {
	t.Helper()
	if os.Getenv("CLOAKBROWSER_WS_URL") == "" && os.Getenv("INTEGRATION") == "" && !chromiumAvailable() {
		t.Skip("no local Chromium found; set INTEGRATION or install chromium")
	}
	l := launcher.New().Headless(true).
		Set("no-sandbox").
		Set("disable-dev-shm-usage")
	if bin := localChromiumBin(); bin != "" {
		l = l.Bin(bin)
	}
	controlURL, err := l.Launch()
	if err != nil {
		t.Skipf("launch dedicated chromium: %v", err)
	}
	br := rod.New().ControlURL(controlURL)
	if err := br.Connect(); err != nil {
		l.Cleanup()
		t.Skipf("connect dedicated chromium: %v", err)
	}
	return br, func() {
		_ = br.Close()
		l.Cleanup()
	}
}

// TestContextPool_DefaultDiscovery_ExcludesIncognitoContext reproduces the
// production bug where mode=default harvests intermittently land in an INCOGNITO
// context instead of the persistent authenticated Chrome profile.
//
// Root cause: getOrCreateContextSafe and rediscoverDefaultContext bound the
// default context's BrowserContextID to the FIRST page target returned by
// TargetGetTargets. When the pool has itself created private/proxy incognito
// contexts (each with live scraping page targets), and TargetGetTargets returns
// them in arbitrary order, that "first page" could be an incognito tab — binding
// mode=default to an incognito, anonymous cookie jar.
//
// This test forces the pre-fix failure deterministically: the ONLY page target
// during discovery is the incognito one, so "first page target wins" is
// guaranteed to pick it. The fix must EXCLUDE any BrowserContextID owned by a
// pool-created (non-default) context, so the default context is never bound to
// an incognito ID.
//
// Pre-fix: mc.ID == incognitoID (bug reproduced).
// Post-fix: mc.ID != incognitoID (no non-incognito page target exists, so the
// empty-ID "land in current default" fallback is taken instead).
func TestContextPool_DefaultDiscovery_ExcludesIncognitoContext(t *testing.T) {
	br, cleanup := launchDedicatedBrowser(t)
	defer cleanup()

	p := NewContextPool(br)
	defer p.Close()

	// Create a REAL incognito (private) BrowserContext with a live page target —
	// exactly what the pool does for mode=private / mode=proxy scraping tabs.
	res, err := proto.TargetCreateBrowserContext{DisposeOnDetach: true}.Call(br)
	if err != nil {
		t.Fatalf("create incognito browser context: %v", err)
	}
	incognitoID := res.BrowserContextID
	if incognitoID == "" {
		t.Fatal("incognito BrowserContextID is empty; cannot distinguish from default")
	}
	if _, err := (proto.TargetCreateTarget{URL: "about:blank", BrowserContextID: incognitoID}).Call(br); err != nil {
		t.Fatalf("create incognito page target: %v", err)
	}

	// Register the incognito context in the pool so it is part of the known,
	// pool-created set the fix must exclude — mirrors getOrCreateContextSafe's
	// p.contexts["private"] = mc for mode != "default".
	p.contextsMu.Lock()
	p.contexts["private"] = &ManagedContext{Mode: "private", ID: incognitoID, Pages: map[string]*ManagedPage{}}
	p.contextsMu.Unlock()

	// Close every page target that is NOT in the incognito context, so the default
	// context contributes zero page targets. Now the incognito page is the ONLY
	// page target, making the pre-fix "first page target wins" loop deterministic.
	closeForeignPageTargets(t, br, incognitoID)

	// Path 1: getOrCreateContextSafe (fresh default-context discovery).
	mc, err := p.getOrCreateContextSafe("default", "default", "")
	if err != nil {
		t.Fatalf("getOrCreateContextSafe(default): %v", err)
	}
	mc.Mu.Lock()
	gotCreate := mc.ID
	mc.Mu.Unlock()
	if gotCreate == incognitoID {
		t.Errorf("getOrCreateContextSafe bound the default context to incognito ID %q; "+
			"pool-created contexts must be excluded from default discovery", incognitoID)
	}

	// Path 2: rediscoverDefaultContext (stale-default recovery re-discovery).
	// Stamp a fake stale ID, then re-discover; the invariant must hold here too.
	mc.Mu.Lock()
	mc.ID = "STALE-DEFAULT-CTX-ID"
	mc.Mu.Unlock()
	p.rediscoverDefaultContext(mc)
	mc.Mu.Lock()
	gotRedisc := mc.ID
	mc.Mu.Unlock()
	if gotRedisc == incognitoID {
		t.Errorf("rediscoverDefaultContext bound the default context to incognito ID %q; "+
			"pool-created contexts must be excluded from default re-discovery", incognitoID)
	}
}

// closeForeignPageTargets closes every page target whose BrowserContextID differs
// from keepID, leaving only keepID's page(s). Because the incognito page is
// created first, at least one target always remains, so Chrome stays alive.
func closeForeignPageTargets(t *testing.T, br *rod.Browser, keepID proto.BrowserBrowserContextID) {
	t.Helper()
	targets, err := proto.TargetGetTargets{}.Call(br)
	if err != nil {
		t.Fatalf("TargetGetTargets: %v", err)
	}
	for _, tinfo := range targets.TargetInfos {
		if tinfo.Type == "page" && tinfo.BrowserContextID != keepID {
			_, _ = proto.TargetCloseTarget{TargetID: tinfo.TargetID}.Call(br)
		}
	}
}
