package browser

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

func TestDoWaitStable_SettlesOnQuietPage(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	br := acquireSharedBrowser(t)
	page, _ := br.Page(proto.TargetCreateTarget{URL: "data:text/html,<h1>ok</h1>"})
	defer func() { _ = page.Close() }()

	dc := &dispatchContext{ctx: context.Background(), page: page}
	start := time.Now()
	err := doWaitStable(dc, Action{QuietMs: 500, MaxWaitMs: 3000})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 450*time.Millisecond {
		t.Errorf("returned too fast (%v) — quiet_ms not enforced", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("returned too slow (%v) — should settle quickly on static page", elapsed)
	}
}

func TestDoWaitStable_TimesOutOnBusyPage(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	br := acquireSharedBrowser(t)
	// Page with a setInterval that fires XHR every 100ms — never settles.
	// Use absolute URL so the request is actually dispatched (relative paths
	// on data: URLs fail before producing NetworkRequestWillBeSent).
	page, _ := br.Page(proto.TargetCreateTarget{URL: "data:text/html,<script>setInterval(()=>fetch('https://example.invalid/x').catch(()=>{}),100)</script>"})
	defer func() { _ = page.Close() }()

	dc := &dispatchContext{ctx: context.Background(), page: page}
	start := time.Now()
	err := doWaitStable(dc, Action{QuietMs: 500, MaxWaitMs: 1500})
	elapsed := time.Since(start)
	if err == nil {
		t.Error("expected timeout error")
	}
	if elapsed < 1400*time.Millisecond {
		t.Errorf("returned too fast (%v)", elapsed)
	}
}

func TestDoWaitStable_IgnoresAnalyticsByDefault(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	br := acquireSharedBrowser(t)
	// Page that fires GA every 200ms — should still settle under default ignores.
	html := `<html><body><script>
	  setInterval(() => fetch('https://www.google-analytics.com/collect?v=1&tid=x').catch(()=>{}), 200);
	</script><h1>hi</h1></body></html>`
	url := "data:text/html," + url.QueryEscape(html)
	page, _ := br.Page(proto.TargetCreateTarget{URL: url})
	defer func() { _ = page.Close() }()

	dc := &dispatchContext{ctx: context.Background(), page: page}
	start := time.Now()
	err := doWaitStable(dc, Action{QuietMs: 500, MaxWaitMs: 5000})
	elapsed := time.Since(start)
	if err != nil {
		t.Errorf("should have settled despite GA noise: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("too slow (%v) — ignore list not applied", elapsed)
	}
}
