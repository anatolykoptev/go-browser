package browser

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// contextKey returns the map key for the given mode/proxy combination.
func contextKey(mode, proxy string) string {
	switch mode {
	case "default":
		return "default"
	case "proxy":
		return "proxy:" + proxy
	default:
		return "private"
	}
}

func (p *ContextPool) getOrCreateContext(key, mode, proxy string) (*ManagedContext, error) {
	if mc, ok := p.contexts[key]; ok {
		return mc, nil
	}

	mc := &ManagedContext{
		Mode:  mode,
		Proxy: proxy,
		Pages: make(map[string]*ManagedPage),
	}

	if mode == "default" {
		// Default context uses empty BrowserContextID — Chrome's persistent profile.
		p.contexts[key] = mc
		return mc, nil
	}

	// Create new incognito/proxy BrowserContext.
	proxyServer, _, _ := parseProxy(proxy)
	res, err := proto.TargetCreateBrowserContext{
		ProxyServer:     proxyServer,
		DisposeOnDetach: true,
	}.Call(p.browser)
	if err != nil {
		return nil, fmt.Errorf("create browser context: %w", err)
	}
	mc.ID = res.BrowserContextID
	p.contexts[key] = mc
	return mc, nil
}

func (p *ContextPool) newPageInContext(mc *ManagedContext) (*rod.Page, error) {
	var targetReq proto.TargetCreateTarget
	targetReq.URL = "about:blank"
	if mc.ID != "" {
		targetReq.BrowserContextID = mc.ID
	}
	res, err := targetReq.Call(p.browser)
	if err != nil {
		return nil, fmt.Errorf("create target: %w", err)
	}
	page, err := p.browser.PageFromTarget(res.TargetID)
	if err != nil {
		return nil, fmt.Errorf("page from target: %w", err)
	}
	return page, nil
}

// adoptExistingPage finds Chrome's existing default tab (e.g. chrome://newtab)
// and wraps it as a rod.Page so the pool can manage it instead of creating a new one.
func (p *ContextPool) adoptExistingPage(mc *ManagedContext) (*rod.Page, error) {
	targets, err := proto.TargetGetTargets{}.Call(p.browser)
	if err != nil {
		return nil, err
	}
	for _, t := range targets.TargetInfos {
		if t.Type != "page" || t.BrowserContextID != mc.ID {
			continue
		}
		page, err := p.browser.PageFromTarget(t.TargetID)
		if err != nil {
			continue
		}
		return page, nil
	}
	return nil, fmt.Errorf("no existing page found")
}

func (p *ContextPool) disposeContext(mc *ManagedContext) {
	if mc.ID != "" {
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: mc.ID}.Call(p.browser)
	}
}

func (p *ContextPool) reaper() {
	defer close(p.done)
	ticker := time.NewTicker(contextPoolReaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.Reap()
		}
	}
}

// watchTargetDestroyed listens for CDP target destruction events and removes
// closed pages from the pool (e.g. when user closes a tab in VNC).
func (p *ContextPool) watchTargetDestroyed() {
	go p.browser.EachEvent(func(e *proto.TargetTargetDestroyed) bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		for key, mc := range p.contexts {
			for name, mp := range mc.Pages {
				if mp.Page != nil && mp.Page.TargetID == e.TargetID {
					delete(mc.Pages, name)
					if len(mc.Pages) == 0 && key != "default" {
						p.disposeContext(mc)
						delete(p.contexts, key)
					}
					return false // keep listening
				}
			}
		}
		return false // keep listening
	})()
}

// formatAge formats the duration since t as a short human-readable string.
func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}
