// Package rod provides a Browser backend using go-rod/rod (in-process Chromium).
package rod

import (
	"context"
	"fmt"
	"log/slog"

	browser "github.com/anatolykoptev/go-browser"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// Browser implements browser.Browser using Rod.
type Browser struct {
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
	if b.rod == nil {
		return nil, browser.ErrUnavailable
	}

	release, err := b.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	renderCtx, cancel := context.WithTimeout(ctx, b.opts.RenderTimeout)
	defer cancel()

	page, err := b.rod.Context(renderCtx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", browser.ErrNavigate, err)
	}
	defer page.Close()

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

	info, _ := page.Info()
	title := ""
	if info != nil {
		title = info.Title
	}

	return &browser.Page{
		URL:   page.MustInfo().URL,
		HTML:  html,
		Title: title,
	}, nil
}

// Available reports whether the Rod browser is connected.
func (b *Browser) Available() bool {
	return b.rod != nil
}

// Close shuts down Rod and the Chromium process.
func (b *Browser) Close() error {
	b.pool.Close()
	if b.rod != nil {
		return b.rod.Close()
	}
	return nil
}
