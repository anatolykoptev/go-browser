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
}

// SessionPool is a thread-safe pool of sessions with TTL-based eviction.
type SessionPool struct {
	mu            sync.Mutex
	sessions      map[string]*Session
	ttl           time.Duration
	maxConcurrent int
	stop          chan struct{}
	done          chan struct{}
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
// Returns an error if the pool is at capacity.
func (p *SessionPool) Create(proxy string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

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
	}
	return id, nil
}

// Get returns the session with the given ID, updating LastUsed.
// Returns false if the session does not exist.
func (p *SessionPool) Get(id string) (*Session, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	s, ok := p.sessions[id]
	if !ok {
		return nil, false
	}
	s.LastUsed = time.Now()
	return s, true
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

// Count returns the number of active sessions.
func (p *SessionPool) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sessions)
}

// Close stops the reaper and destroys all sessions.
func (p *SessionPool) Close() {
	close(p.stop)
	<-p.done

	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessions = make(map[string]*Session)
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
		if time.Since(s.LastUsed) > p.ttl {
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
