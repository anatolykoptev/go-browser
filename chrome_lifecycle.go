package browser

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// getBrowser returns the current browser under a read lock.
func (m *ChromeManager) getBrowser() *rod.Browser {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.browser
}

// reconnect closes the old connection and establishes a new one.
func (m *ChromeManager) reconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.browser != nil {
		_ = m.browser.Close()
		m.browser = nil
	}
	m.keepaliveCtxID = ""

	debuggerURL, err := discoverWSURL(m.wsURL)
	if err != nil {
		return fmt.Errorf("discover ws url: %w", err)
	}

	b := rod.New().ControlURL(debuggerURL)
	if err := b.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	m.browser = b
	m.keepaliveCtxID = ""
	if m.pool != nil {
		m.pool.UpdateBrowser(b)
	} else {
		m.pool = NewContextPool(b)
	}
	return nil
}

// Connected reports whether the Chrome connection is active.
func (m *ChromeManager) Connected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.browser != nil
}

// Close disconnects from Chrome and releases resources.
func (m *ChromeManager) Close() {
	m.mu.Lock()
	b := m.browser
	m.browser = nil
	ctxID := m.keepaliveCtxID
	m.keepaliveCtxID = ""
	m.mu.Unlock()

	if b != nil {
		if ctxID != "" {
			_ = proto.TargetDisposeBrowserContext{BrowserContextID: ctxID}.Call(b)
		}
		_ = b.Close()
	}
}

// DefaultContext returns the browser's default context (persistent profile).
// Cookies, localStorage, and other state from manual login sessions are available.
// No isolation — all requests share the same cookies.
func (m *ChromeManager) DefaultContext() (*rod.Browser, error) {
	b := m.getBrowser()
	if b == nil {
		return nil, ErrUnavailable
	}
	scoped := b.NoDefaultDevice()
	scoped.BrowserContextID = "" // empty = default context (persistent profile)
	return scoped, nil
}

// PageInfo describes an open browser page/tab.
type PageInfo struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Context string `json:"context"` // "default", "keepalive", or context ID
}

// ListPages returns all open pages/tabs with their context info.
func (m *ChromeManager) ListPages() ([]PageInfo, error) {
	b := m.getBrowser()
	if b == nil {
		return nil, ErrUnavailable
	}
	targets, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	var pages []PageInfo
	for _, t := range targets.TargetInfos {
		if t.Type != "page" {
			continue
		}
		ctx := "default"
		if t.BrowserContextID != "" {
			if t.BrowserContextID == m.keepaliveCtxID {
				ctx = "keepalive"
			} else {
				ctx = string(t.BrowserContextID)
			}
		}
		pages = append(pages, PageInfo{
			ID:      string(t.TargetID),
			Type:    string(t.Type),
			URL:     t.URL,
			Title:   t.Title,
			Context: ctx,
		})
	}
	return pages, nil
}

