package humanize_test

import (
	"testing"

	"github.com/anatolykoptev/go-browser/humanize"
)

func TestCursor_InitialPosition(t *testing.T) {
	c := humanize.NewCursor(100, 200)
	x, y := c.Position()
	if x != 100 || y != 200 {
		t.Errorf("got (%v,%v), want (100,200)", x, y)
	}
}

func TestCursor_MoveTo(t *testing.T) {
	c := humanize.NewCursor(0, 0)
	c.MoveTo(50.5, 75.3)
	x, y := c.Position()
	if x != 50.5 || y != 75.3 {
		t.Errorf("got (%v,%v), want (50.5,75.3)", x, y)
	}
}
