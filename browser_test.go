package browser_test

import (
	"testing"
	"time"

	"github.com/anatolykoptev/go-browser"
)

func TestDefaultOptions(t *testing.T) {
	o := browser.DefaultOptions()
	if o.Concurrency != 3 {
		t.Errorf("Concurrency = %d, want 3", o.Concurrency)
	}
	if o.RenderTimeout != 20*time.Second {
		t.Errorf("RenderTimeout = %v, want 20s", o.RenderTimeout)
	}
	if o.HydrationWait != 2*time.Second {
		t.Errorf("HydrationWait = %v, want 2s", o.HydrationWait)
	}
}

func TestApply_NoOpts_ReturnsDefaults(t *testing.T) {
	o := browser.Apply()
	def := browser.DefaultOptions()
	if o.Concurrency != def.Concurrency {
		t.Errorf("Concurrency mismatch")
	}
}
