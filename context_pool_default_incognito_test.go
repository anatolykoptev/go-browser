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
	incognitoID := newContextWithPage(t, br)

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

// closeForeignPageTargets closes every page target whose BrowserContextID is not
// in keepIDs, leaving only those contexts' page(s). Callers create the kept
// page(s) first, so at least one target always remains and Chrome stays alive.
func closeForeignPageTargets(t *testing.T, br *rod.Browser, keepIDs ...proto.BrowserBrowserContextID) {
	t.Helper()
	keep := make(map[proto.BrowserBrowserContextID]struct{}, len(keepIDs))
	for _, id := range keepIDs {
		keep[id] = struct{}{}
	}
	targets, err := proto.TargetGetTargets{}.Call(br)
	if err != nil {
		t.Fatalf("TargetGetTargets: %v", err)
	}
	for _, tinfo := range targets.TargetInfos {
		if tinfo.Type != "page" {
			continue
		}
		if _, kept := keep[tinfo.BrowserContextID]; kept {
			continue
		}
		_, _ = proto.TargetCloseTarget{TargetID: tinfo.TargetID}.Call(br)
	}
}

// newContextWithPage creates a real BrowserContext with one live page target and
// returns its ID — a stand-in for either a pool-created incognito context or an
// unmanaged "persistent default"-like context, depending on whether the caller
// registers it in the pool.
func newContextWithPage(t *testing.T, br *rod.Browser) proto.BrowserBrowserContextID {
	t.Helper()
	res, err := proto.TargetCreateBrowserContext{DisposeOnDetach: true}.Call(br)
	if err != nil {
		t.Fatalf("create browser context: %v", err)
	}
	if res.BrowserContextID == "" {
		t.Fatal("created BrowserContextID is empty")
	}
	if _, err := (proto.TargetCreateTarget{URL: "about:blank", BrowserContextID: res.BrowserContextID}).Call(br); err != nil {
		t.Fatalf("create page target in %q: %v", res.BrowserContextID, err)
	}
	return res.BrowserContextID
}

// TestContextPool_DefaultDiscovery_PicksPersistentDefault asserts the POSITIVE
// contract: when a persistent-default-like page target (a context the pool did
// NOT create) coexists with a pool-created incognito page target, discovery must
// return the PERSISTENT default's ID — not the incognito one, and not the empty
// fallback. This guards against an impl that "passes" the exclusion test merely
// by always returning "" (which would never reach the authenticated profile),
// and pins the MEDIUM GetTargets-before-snapshot reorder against silently
// regressing to that empty fallback. Verified for BOTH discovery paths.
func TestContextPool_DefaultDiscovery_PicksPersistentDefault(t *testing.T) {
	br, cleanup := launchDedicatedBrowser(t)
	defer cleanup()

	p := NewContextPool(br)
	defer p.Close()

	// An unmanaged context with a live page — the pool never created it, so from
	// the pool's view it is the ambient/persistent default (mirrors the operator's
	// login tab, which lives in a context NOT in the managed pool).
	persistentID := newContextWithPage(t, br)

	// A pool-created incognito context, registered exactly as getOrCreateContextSafe
	// would (p.contexts["private"] = mc for mode != "default").
	incognitoID := newContextWithPage(t, br)
	if persistentID == incognitoID {
		t.Fatal("persistent and incognito contexts collided")
	}
	p.contextsMu.Lock()
	p.contexts["private"] = &ManagedContext{Mode: "private", ID: incognitoID, Pages: map[string]*ManagedPage{}}
	p.contextsMu.Unlock()

	// Leave only the persistent + incognito page targets; the persistent one is the
	// sole non-incognito page, so discovery must select it regardless of order.
	closeForeignPageTargets(t, br, persistentID, incognitoID)

	// Path 1: getOrCreateContextSafe.
	mc, err := p.getOrCreateContextSafe("default", "default", "")
	if err != nil {
		t.Fatalf("getOrCreateContextSafe(default): %v", err)
	}
	mc.Mu.Lock()
	gotCreate := mc.ID
	mc.Mu.Unlock()
	if gotCreate != persistentID {
		t.Errorf("getOrCreateContextSafe bound default to %q; want persistent default %q (incognito was %q)",
			gotCreate, persistentID, incognitoID)
	}

	// Path 2: rediscoverDefaultContext.
	mc.Mu.Lock()
	mc.ID = "STALE-DEFAULT-CTX-ID"
	mc.Mu.Unlock()
	p.rediscoverDefaultContext(mc)
	mc.Mu.Lock()
	gotRedisc := mc.ID
	mc.Mu.Unlock()
	if gotRedisc != persistentID {
		t.Errorf("rediscoverDefaultContext bound default to %q; want persistent default %q (incognito was %q)",
			gotRedisc, persistentID, incognitoID)
	}
}
