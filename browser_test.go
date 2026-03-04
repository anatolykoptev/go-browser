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

func TestApply_OverridesDefaults(t *testing.T) {
	o := browser.Apply(
		browser.WithConcurrency(5),
		browser.WithRenderTimeout(10*time.Second),
		browser.WithHydrationWait(1*time.Second),
		browser.WithUserAgent("test-ua"),
	)
	if o.Concurrency != 5 {
		t.Errorf("Concurrency = %d, want 5", o.Concurrency)
	}
	if o.RenderTimeout != 10*time.Second {
		t.Errorf("RenderTimeout = %v, want 10s", o.RenderTimeout)
	}
	if o.HydrationWait != 1*time.Second {
		t.Errorf("HydrationWait = %v, want 1s", o.HydrationWait)
	}
	if o.UserAgent != "test-ua" {
		t.Errorf("UserAgent = %q, want 'test-ua'", o.UserAgent)
	}
}

func TestApply_NoOpts_ReturnsDefaults(t *testing.T) {
	o := browser.Apply()
	def := browser.DefaultOptions()
	if o.Concurrency != def.Concurrency {
		t.Errorf("Concurrency mismatch")
	}
}
