package rod_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anatolykoptev/go-browser"
	rodbackend "github.com/anatolykoptev/go-browser/rod"
)

func TestBrowser_ZeroValue_NotAvailable(t *testing.T) {
	var b rodbackend.Browser
	if b.Available() {
		t.Error("zero-value browser should not be available")
	}
}

func TestBrowser_ZeroValue_RenderReturnsUnavailable(t *testing.T) {
	var b rodbackend.Browser
	_, err := b.Render(context.Background(), "https://example.com")
	if !errors.Is(err, browser.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestBrowser_ZeroValue_CloseNoPanic(t *testing.T) {
	var b rodbackend.Browser
	if err := b.Close(); err != nil {
		t.Errorf("close zero-value: %v", err)
	}
}

func TestRender_EmptyURL_ReturnsError(t *testing.T) {
	var b rodbackend.Browser
	_, err := b.Render(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	if !errors.Is(err, browser.ErrNavigate) {
		t.Errorf("err = %v, want ErrNavigate", err)
	}
}

func TestDefaultOptions(t *testing.T) {
	o := rodbackend.DefaultOptions()
	if !o.Headless {
		t.Error("Headless should default to true")
	}
	if o.Concurrency != 3 {
		t.Errorf("Concurrency = %d, want 3", o.Concurrency)
	}
	if o.Bin != "" {
		t.Errorf("Bin should default to empty")
	}
}

func TestWithOptions(t *testing.T) {
	o := rodbackend.DefaultOptions()
	rodbackend.WithBin("/usr/bin/chromium")(&o)
	rodbackend.WithHeadless(false)(&o)
	if o.Bin != "/usr/bin/chromium" {
		t.Errorf("Bin = %q, want /usr/bin/chromium", o.Bin)
	}
	if o.Headless {
		t.Error("Headless should be false")
	}
}
