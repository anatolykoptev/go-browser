package browser

import (
	"strings"
	"testing"
	"time"
	"github.com/go-rod/rod/lib/proto"
)

func TestLogCollector_RingBufferDropsOldest(t *testing.T) {
	c := NewLogCollector()
	for i := 0; i < maxLogEntries+50; i++ {
		c.AddNetwork(NetworkEntry{URL: "https://x.test/", TS: int64(i)})
	}
	net, _ := c.Collect()
	if len(net) != maxLogEntries {
		t.Fatalf("network len = %d, want %d", len(net), maxLogEntries)
	}
	// Oldest kept should be entry 50, newest should be entry maxLogEntries+49.
	if net[0].TS != 50 {
		t.Errorf("oldest retained TS = %d, want 50 (dropping oldest, not newest)", net[0].TS)
	}
	if net[len(net)-1].TS != int64(maxLogEntries+49) {
		t.Errorf("newest retained TS = %d, want %d", net[len(net)-1].TS, maxLogEntries+49)
	}
}

func TestLogCollector_Since(t *testing.T) {
	c := NewLogCollector()
	c.AddNetwork(NetworkEntry{URL: "a", TS: 100})
	c.AddNetwork(NetworkEntry{URL: "b", TS: 200})
	c.AddConsole(ConsoleEntry{Text: "c1", TS: 150})
	c.AddException(ExceptionEntry{Text: "ex", TS: 180})
	c.AddNavigation(NavigationEntry{URL: "nav", TS: 120})

	s := c.Since(150)
	if len(s.Network) != 1 || s.Network[0].URL != "b" {
		t.Errorf("Since network: got %+v", s.Network)
	}
	if len(s.Console) != 0 {
		t.Errorf("Since console: should exclude ts=150 (exclusive), got %+v", s.Console)
	}
	if len(s.Exceptions) != 1 {
		t.Errorf("Since exceptions: got %+v", s.Exceptions)
	}
	if len(s.Navigations) != 0 {
		t.Errorf("Since navigations: ts=120 is < 150, got %+v", s.Navigations)
	}
}

func TestLogCollector_CapturesExceptionsAndNavigations_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	br := acquireSharedBrowser(t)
	page, _ := br.Page(proto.TargetCreateTarget{URL: "about:blank"})
	defer func() { _ = page.Close() }()

	c := NewLogCollector()
	c.SubscribeCDP(page)

	_ = page.Navigate("data:text/html,<script>throw new Error('boom')</script>")
	_ = page.WaitLoad()
	time.Sleep(500 * time.Millisecond)

	s := c.Since(0)
	if len(s.Exceptions) == 0 {
		t.Error("expected exception to be captured")
	}
	if len(s.Navigations) == 0 {
		t.Error("expected navigation event to be captured")
	}
}

func TestLogCollector_CapturesMainFrameNavigation_Integration(t *testing.T) {
	if testing.Short() { t.Skip("integration") }
	br := acquireSharedBrowser(t)
	page, _ := br.Page(proto.TargetCreateTarget{URL: "about:blank"})
	defer func() { _ = page.Close() }()

	c := NewLogCollector()
	c.SubscribeCDP(page)

	if err := page.Navigate("https://example.com"); err != nil { t.Fatal(err) }
	_ = page.WaitLoad()
	time.Sleep(500 * time.Millisecond)

	res := c.Since(0)
	if len(res.Navigations) == 0 {
		t.Error("expected main-frame navigation event, got none")
	}
	foundExample := false
	for _, n := range res.Navigations {
		if strings.Contains(n.URL, "example.com") { foundExample = true; break }
	}
	if !foundExample {
		t.Errorf("expected example.com navigation, got %+v", res.Navigations)
	}
}

func TestLogCollector_SinceFiltered(t *testing.T) {
	c := NewLogCollector()
	c.AddNetwork(NetworkEntry{URL: "a", Status: 200, TS: 100})
	c.AddNetwork(NetworkEntry{URL: "b", Status: 404, TS: 200})
	c.AddNetwork(NetworkEntry{URL: "c", Status: 500, TS: 300})
	c.AddConsole(ConsoleEntry{Text: "log", TS: 150})

	// Only errors
	r := c.SinceFiltered(0, SinceFilter{Kinds: []string{"network"}, StatusMin: 400})
	if len(r.Network) != 2 { t.Errorf("want 2 error rows, got %d", len(r.Network)) }
	if len(r.Console) != 0 { t.Errorf("console should be filtered out, got %d", len(r.Console)) }

	// Limit
	r = c.SinceFiltered(0, SinceFilter{Limit: 1})
	if len(r.Network) != 1 || len(r.Console) != 1 {
		t.Errorf("limit=1 per category, got net=%d con=%d", len(r.Network), len(r.Console))
	}
}
