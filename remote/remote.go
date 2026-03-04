// Package remote provides a Browser backend connecting to an external CDP endpoint.
// Compatible with Browserless, Lightpanda, or any CDP-compatible server.
package remote

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	browser "github.com/anatolykoptev/go-browser"
	"github.com/chromedp/chromedp"
)

// Browser implements browser.Browser via a remote CDP WebSocket.
type Browser struct {
	ctx         context.Context
	allocCancel context.CancelFunc
	ctxCancel   context.CancelFunc
	pool        *browser.Pool
	opts        Options
	connected   atomic.Bool
}

// New connects to a remote CDP endpoint.
func New(opts ...Option) (*Browser, error) {
	o := Options{Options: browser.DefaultOptions()}
	for _, fn := range opts {
		fn(&o)
	}

	if o.Endpoint == "" {
		return &Browser{pool: browser.NewPool(1)}, nil
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(
		context.Background(), o.Endpoint, chromedp.NoModifyURL,
	)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	if err := chromedp.Run(ctx); err != nil {
		allocCancel()
		ctxCancel()
		return nil, fmt.Errorf("remote: connect %s: %w", o.Endpoint, err)
	}

	slog.Info("remote: connected", "endpoint", o.Endpoint)

	b := &Browser{
		ctx:         ctx,
		allocCancel: allocCancel,
		ctxCancel:   ctxCancel,
		pool:        browser.NewPool(o.Concurrency),
		opts:        o,
	}
	b.connected.Store(true)
	return b, nil
}

// Render navigates to url, waits, returns rendered HTML.
func (b *Browser) Render(ctx context.Context, url string) (*browser.Page, error) {
	if url == "" {
		return nil, fmt.Errorf("%w: empty URL", browser.ErrNavigate)
	}
	if !b.connected.Load() {
		return nil, browser.ErrUnavailable
	}

	release, err := b.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	tabCtx, tabCancel := chromedp.NewContext(b.ctx)
	defer tabCancel()

	renderCtx, renderCancel := context.WithTimeout(tabCtx, b.opts.RenderTimeout)
	defer renderCancel()

	var html string
	err = chromedp.Run(renderCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(b.opts.HydrationWait),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		if renderCtx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", browser.ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: %v", browser.ErrNavigate, err)
	}

	var title, finalURL string
	_ = chromedp.Run(renderCtx, chromedp.Title(&title))
	_ = chromedp.Run(renderCtx, chromedp.Location(&finalURL))
	if finalURL == "" {
		finalURL = url
	}

	return &browser.Page{
		URL:   finalURL,
		HTML:  html,
		Title: title,
	}, nil
}

// Available reports whether the remote browser is connected.
func (b *Browser) Available() bool { return b.connected.Load() }

// Close disconnects from the remote browser.
func (b *Browser) Close() error {
	b.connected.Store(false)
	b.pool.Close()
	if b.ctxCancel != nil {
		b.ctxCancel()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
	return nil
}
