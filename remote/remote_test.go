package remote_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anatolykoptev/go-browser"
	"github.com/anatolykoptev/go-browser/remote"
)

func TestNew_EmptyEndpoint_NotAvailable(t *testing.T) {
	b, err := remote.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = b.Close() }()

	if b.Available() {
		t.Error("empty endpoint should not be available")
	}
}

func TestNew_EmptyEndpoint_RenderReturnsUnavailable(t *testing.T) {
	b, err := remote.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = b.Close() }()

	_, err = b.Render(context.Background(), "https://example.com")
	if !errors.Is(err, browser.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestNew_EmptyEndpoint_CloseNoPanic(t *testing.T) {
	b, err := remote.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Should not panic on nil pool.
	if err := b.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
	// Double close should also be safe.
	if err := b.Close(); err != nil {
		t.Errorf("double close: %v", err)
	}
}

func TestRender_EmptyURL_ReturnsError(t *testing.T) {
	b, err := remote.New(remote.WithEndpoint(""))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = b.Close() }()

	_, err = b.Render(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestNew_InvalidEndpoint_ReturnsError(t *testing.T) {
	_, err := remote.New(remote.WithEndpoint("ws://127.0.0.1:1/nonexistent"))
	if err == nil {
		t.Fatal("expected error connecting to invalid endpoint")
	}
}
