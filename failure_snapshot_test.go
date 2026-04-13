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
