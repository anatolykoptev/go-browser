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

func TestWithBlockResources(t *testing.T) {
	o := rodbackend.DefaultOptions()
	rodbackend.WithBlockResources(
		rodbackend.ResourceImage,
		rodbackend.ResourceFont,
	)(&o)
	if len(o.BlockResources) != 2 {
		t.Fatalf("BlockResources len = %d, want 2", len(o.BlockResources))
	}
	if o.BlockResources[0] != rodbackend.ResourceImage {
		t.Errorf("BlockResources[0] = %q, want Image", o.BlockResources[0])
	}
	if o.BlockResources[1] != rodbackend.ResourceFont {
		t.Errorf("BlockResources[1] = %q, want Font", o.BlockResources[1])
	}
}

func TestResourceTypeConstants(t *testing.T) {
	types := []rodbackend.ResourceType{
		rodbackend.ResourceImage,
		rodbackend.ResourceFont,
		rodbackend.ResourceStylesheet,
		rodbackend.ResourceMedia,
	}
	for _, rt := range types {
		if rt == "" {
			t.Errorf("ResourceType constant is empty")
		}
	}
}
