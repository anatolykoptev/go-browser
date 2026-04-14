package browser

import (
	"strings"
	"testing"
	"time"
)

// TestPool_SubscribesLogCollectorOnCreate verifies that after GetOrCreatePage,
// the managed page's LogCollector has a live CDP subscription and accumulates
// events when the page navigates.
func TestPool_SubscribesLogCollectorOnCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	br := acquireSharedBrowser(t)
	p := NewContextPool(br)
	defer p.Close()

	mp, err := p.GetOrCreatePage("logsub", "private", "", "about:blank")
	if err != nil {
		t.Fatal(err)
	}
	if mp.LogCollector == nil {
		t.Fatal("LogCollector must be non-nil")
	}

	// Drive the page to produce one network + one navigation event.
	_ = mp.Page.Navigate("data:text/html,<script>fetch('https://example.com/')</script><h1>ok</h1>")
	_ = mp.Page.WaitLoad()
	time.Sleep(700 * time.Millisecond) // give event goroutine time

	res := mp.LogCollector.Since(0)
	if len(res.Network) == 0 {
		t.Error("expected at least one network entry, got none — SubscribeCDP never attached")
	}
	// Note: Navigation entries depend on logs.go main-frame detection
	// (FrameID=="") which may not trigger for data: URLs; the real
	// wiring bug we're testing is that *any* events arrive.
	hasExample := false
	for _, n := range res.Network {
		if strings.Contains(n.URL, "example.com") {
			hasExample = true
			break
		}
	}
	if !hasExample {
		t.Errorf("expected example.com in network entries, got %+v", res.Network)
	}
}
