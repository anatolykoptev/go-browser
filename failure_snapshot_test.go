package browser

import (
	"testing"
	"github.com/go-rod/rod/lib/proto"
)

func TestCaptureFailureSnapshot_NilPage(t *testing.T) {
	if got := CaptureFailureSnapshot(nil); got != nil {
		t.Errorf("nil page should return nil snapshot, got %+v", got)
	}
}

// Integration test uses shared test browser.
func TestCaptureFailureSnapshot_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	br := acquireSharedBrowser(t)
	page, err := br.Page(proto.TargetCreateTarget{URL: "data:text/html,<h1>Hello</h1><button>Click</button>"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = page.Close() }()
	// Wait for the target to finish attaching/loading before asserting on
	// its state — TargetCreateTarget returns as soon as the target exists,
	// not once navigation completes, so reading page.Info() immediately is
	// a race (this test shares acquireSharedBrowser's browser with every
	// other integration test in the package, including the egress guard's
	// own tests, which install a Fetch-domain listener that adds a little
	// CDP round-trip latency to every target's lifecycle — enough to flip
	// this previously-racy assertion depending on test order).
	if err := page.WaitLoad(); err != nil {
		t.Fatalf("WaitLoad: %v", err)
	}

	fs := CaptureFailureSnapshot(page)
	if fs == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if fs.URL == "" {
		t.Error("URL should be populated")
	}
	if fs.ScreenshotB64 == "" {
		t.Error("screenshot should be populated")
	}
	if len(fs.Snapshot) == 0 {
		t.Error("AXTree snapshot should be populated")
	}
}
