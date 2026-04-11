package browser

import (
	"time"

	"github.com/go-rod/rod"
)

// GetPage returns the stored page for a session, or nil if not found/expired.
func (p *SessionPool) GetPage(id string) *rod.Page {
	p.mu.Lock()
	defer p.mu.Unlock()
	sess, ok := p.sessions[id]
	if !ok || sess.isExpired() {
		return nil
	}
	sess.LastUsed = time.Now()
	return sess.Page
}

// StorePage associates a page with an existing session.
func (p *SessionPool) StorePage(id string, page *rod.Page) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if sess, ok := p.sessions[id]; ok {
		sess.Page = page
	}
}
