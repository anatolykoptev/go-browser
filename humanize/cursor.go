package humanize

import "sync"

// Cursor tracks the current mouse position for humanized movement.
// Safe for concurrent use.
type Cursor struct {
	mu   sync.Mutex
	x, y float64
}

// NewCursor creates a cursor at the given position.
func NewCursor(x, y float64) *Cursor {
	return &Cursor{x: x, y: y}
}

// Position returns the current cursor position.
func (c *Cursor) Position() (float64, float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.x, c.y
}

// MoveTo updates the cursor position.
func (c *Cursor) MoveTo(x, y float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.x = x
	c.y = y
}
