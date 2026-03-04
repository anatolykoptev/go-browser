// Package rod provides a Browser backend using go-rod/rod (in-process Chromium).
package rod

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	browser "github.com/anatolykoptev/go-browser"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// Browser implements browser.Browser using Rod.
type Browser struct {
	mu   sync.RWMutex
	rod  *rod.Browser
	pool *browser.Pool
	opts Options
}

// New creates a Rod browser backend.
func New(opts ...Option) (*Browser, error) {
	o := DefaultOptions()
	for _, fn := range opts {
		fn(&o)
	}

	l := launcher.New().Headless(o.Headless)
	if o.Bin != "" {
		l = l.Bin(o.Bin)
	}
	if o.ProxyPool != nil {
		if proxy := o.ProxyPool.Next(); proxy != "" {
			l = l.Proxy(proxy)
		}
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("rod: launch: %w", err)
	}

	b := rod.New().ControlURL(controlURL)
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("rod: connect: %w", err)
	}

	slog.Info("rod: browser launched", "headless", o.Headless)

	return &Browser{
		rod:  b,
		pool: browser.NewPool(o.Concurrency),
		opts: o,
	}, nil
}

// Render navigates to url, waits for JS, returns rendered HTML.
func (b *Browser) Render(ctx context.Context, url string) (*browser.Page, error) {
	if url == "" {
		return nil, fmt.Errorf("%w: empty URL", browser.ErrNavigate)
	}

	b.mu.RLock()
	r := b.rod
	b.mu.RUnlock()

	if r == nil {
		return nil, browser.ErrUnavailable
	}

	release, err := b.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	renderCtx, cancel := context.WithTimeout(ctx, b.opts.RenderTimeout)
	defer cancel()

	page, err := r.Context(renderCtx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", browser.ErrNavigate, err)
	}
	defer func() {
		if closeErr := page.Close(); closeErr != nil {
			slog.Warn("rod: page close failed", "err", closeErr)
		}
	}()

	if err := page.Navigate(url); err != nil {
		return nil, fmt.Errorf("%w: %v", browser.ErrNavigate, err)
	}

	if err := page.WaitDOMStable(b.opts.HydrationWait, 0.1); err != nil {
		return nil, fmt.Errorf("%w: %v", browser.ErrTimeout, err)
	}

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("rod: html: %w", err)
	}

	// Get page info safely (no MustInfo panic).
	finalURL := url
	title := ""
	if info, infoErr := page.Info(); infoErr == nil && info != nil {
		finalURL = info.URL
		title = info.Title
	}

	return &browser.Page{
		URL:   finalURL,
		HTML:  html,
		Title: title,
	}, nil
}

// Available reports whether the Rod browser is connected.
func (b *Browser) Available() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.rod != nil
}

// Close shuts down Rod and the Chromium process.
func (b *Browser) Close() error {
	b.mu.Lock()
	r := b.rod
	b.rod = nil
	b.mu.Unlock()

	if b.pool != nil {
		b.pool.Close()
	}
	if r != nil {
		return r.Close()
	}
	return nil
}
