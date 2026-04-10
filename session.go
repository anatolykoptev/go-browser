package browser

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const (
	reaperInterval = 30 * time.Second
	sessionIDBytes = 16
)

// Session represents a browser session with lifecycle metadata.
type Session struct {
	ID        string
	CreatedAt time.Time
	LastUsed  time.Time
	Proxy     string
	ttl       time.Duration // 0 means never expires; set from pool at creation time
}

// isExpired reports whether the session has been idle longer than its TTL.
// A zero TTL means the session never expires.
func (s *Session) isExpired() bool {
	if s.ttl <= 0 {
		return false
	}
	return time.Since(s.LastUsed) > s.ttl
}

// SessionPool is a thread-safe pool of sessions with TTL-based eviction.
type SessionPool struct {
	mu            sync.Mutex
	sessions      map[string]*Session
	ttl           time.Duration
	maxConcurrent int
	stop          chan struct{}
	done          chan struct{}
	closed        bool
}

// NewSessionPool creates a SessionPool with the given TTL and max concurrent sessions.
// A background reaper goroutine is started immediately.
func NewSessionPool(ttl time.Duration, maxConcurrent int) *SessionPool {
	p := &SessionPool{
		sessions:      make(map[string]*Session),
		ttl:           ttl,
		maxConcurrent: maxConcurrent,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	go p.reaper()
	return p
}

// Create allocates a new session with the given proxy and returns its ID.
// Returns an error if the pool is at capacity or has been closed.
func (p *SessionPool) Create(proxy string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return "", fmt.Errorf("session pool is closed")
	}
	if p.maxConcurrent > 0 && len(p.sessions) >= p.maxConcurrent {
		return "", fmt.Errorf("session pool at capacity (%d)", p.maxConcurrent)
	}

	id, err := generateID()
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	now := time.Now()
	p.sessions[id] = &Session{
		ID:        id,
		CreatedAt: now,
		LastUsed:  now,
		Proxy:     proxy,
		ttl:       p.ttl,
	}
	return id, nil
}

// Get returns the session with the given ID, updating LastUsed.
// Returns an error if the pool is closed, the session does not exist, or has expired.
// Expired sessions are eagerly removed from the pool without waiting for the reaper.
func (p *SessionPool) Get(id string) (*Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("session pool is closed")
	}
	s, ok := p.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found (may have expired)", id)
	}
	if s.isExpired() {
		delete(p.sessions, id)
		return nil, fmt.Errorf("session %q expired (TTL exceeded)", id)
	}
	s.LastUsed = time.Now()
	return s, nil
}

// Destroy removes the session with the given ID. Returns true if it existed.
func (p *SessionPool) Destroy(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, ok := p.sessions[id]
	if ok {
		delete(p.sessions, id)
	}
	return ok
}

// List returns a copy of all active sessions.
func (p *SessionPool) List() []Session {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]Session, 0, len(p.sessions))
	for _, s := range p.sessions {
		result = append(result, *s)
	}
	return result
}

// Count returns the number of active sessions.
func (p *SessionPool) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sessions)
}

// Close stops the reaper and destroys all sessions.
// After Close, Create and Get return errors.
func (p *SessionPool) Close() {
	close(p.stop)
	<-p.done

	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessions = make(map[string]*Session)
	p.closed = true
}

// reaper runs in the background, evicting sessions idle longer than ttl.
func (p *SessionPool) reaper() {
	defer close(p.done)

	ticker := time.NewTicker(reaperInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.evictExpired()
		}
	}
}

func (p *SessionPool) evictExpired() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, s := range p.sessions {
		if s.isExpired() {
			delete(p.sessions, id)
		}
	}
}

func generateID() (string, error) {
	b := make([]byte, sessionIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
