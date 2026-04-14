package browser

import (
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// TestModifierBitmask verifies the CDP Input.dispatchKeyEvent bitmask mapping.
// Alt=1, Control=2, Meta=4, Shift=8 — combinations OR together.
func TestModifierBitmask(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want int
	}{
		{"none", nil, 0},
		{"alt", []string{"Alt"}, 1},
		{"control", []string{"Control"}, 2},
		{"meta", []string{"Meta"}, 4},
		{"shift", []string{"Shift"}, 8},
		{"ctrl_shift", []string{"Control", "Shift"}, 10},
		{"all", []string{"Alt", "Control", "Meta", "Shift"}, 15},
		{"unknown_ignored", []string{"Hyper", "Control"}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := modifierBitmask(c.in); got != c.want {
				t.Errorf("modifierBitmask(%v) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// TestDoPress_EmptyKey verifies empty key is rejected without a browser.
func TestDoPress_EmptyKey(t *testing.T) {
	err := doPress(nil, "", nil)
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "empty key") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDoPress_UnsupportedKey verifies multi-rune strings not in keyMap fail
// with a clear message (protects against callers passing "Control+a" etc.).
func TestDoPress_UnsupportedKey(t *testing.T) {
	err := doPress(nil, "Control+a", nil)
	if err == nil {
		t.Fatal("expected error for multi-char unknown key")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestPress_LetterWithCtrl verifies that press with modifiers={Control} and
// key="a" issues Ctrl+A (select-all) to the focused contenteditable element,
// followed by press Backspace to delete the selected text. After the sequence
// the innerText must be empty.
//
// Regression guard for v0.15.0: the old doPress rejected single letters with
// "unknown key".
func TestPress_LetterWithCtrl(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	b := acquireSharedBrowser(t)

	// Use a plain <input> — more reliable than contenteditable across
	// stealth-patched browsers. Ctrl+A+Backspace must empty its value.
	const html = `data:text/html,<input id="t" value="hello world" autofocus>`
	page, err := b.Page(proto.TargetCreateTarget{URL: html})
	if err != nil {
		t.Fatalf("open page: %v", err)
	}
	defer func() { _ = page.Close() }()
	_ = page.Timeout(5 * time.Second).WaitLoad()

	el, err := page.Timeout(5 * time.Second).Element("#t")
	if err != nil {
		t.Fatalf("find #t: %v", err)
	}
	if err := el.Focus(); err != nil {
		t.Fatalf("focus: %v", err)
	}

	// Ctrl+A (select all)
	if err := doPress(page, "a", []string{"Control"}); err != nil {
		t.Fatalf("doPress Ctrl+A: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	// Backspace (delete selection)
	if err := doPress(page, "Backspace", nil); err != nil {
		t.Fatalf("doPress Backspace: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	out, err := el.Eval(`function(){ return this.value; }`)
	if err != nil {
		t.Fatalf("read value: %v", err)
	}
	got := strings.TrimSpace(out.Value.Str())
	if got != "" {
		t.Errorf("after Ctrl+A+Backspace value = %q, want empty", got)
	}
}

// TestPress_NamedKeyStillWorks guards the non-regression of named-key path.
func TestPress_NamedKeyStillWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	b := acquireSharedBrowser(t)

	page, err := b.Page(proto.TargetCreateTarget{URL: `data:text/html,<input id="i" autofocus>`})
	if err != nil {
		t.Fatalf("open page: %v", err)
	}
	defer func() { _ = page.Close() }()
	_ = page.Timeout(5 * time.Second).WaitLoad()

	el, err := page.Timeout(5 * time.Second).Element("#i")
	if err != nil {
		t.Fatalf("find #i: %v", err)
	}
	if err := el.Focus(); err != nil {
		t.Fatalf("focus: %v", err)
	}
	if err := doPress(page, "Enter", nil); err != nil {
		t.Errorf("doPress Enter: %v", err)
	}
}
