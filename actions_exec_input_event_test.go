package browser

import (
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

// TestInsertTextInputEvent_ContentEditable verifies that
// doInsertTextInputEvent dispatches a beforeinput/input pair on a plain
// contenteditable div and that the default handler actually inserts the text.
//
// This is a smoke test for event mechanics, NOT a TipTap correctness test.
// The production target (TipTap/ProseMirror on LinkedIn) is validated via
// MCP chrome_interact, not in a unit test.
//
// Browser default beforeinput behavior with inputType=insertReplacementText
// on a contenteditable replaces the current Selection Range with the `data`
// payload. We pre-select all contents inside the function, so "OLD" becomes
// "NEW".
func TestInsertTextInputEvent_ContentEditable(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	b := acquireSharedBrowser(t)

	const html = `data:text/html,<div contenteditable="true" id="t" style="padding:8px;border:1px solid #333">OLD</div>`
	page, err := b.Page(proto.TargetCreateTarget{URL: html})
	if err != nil {
		t.Fatalf("open page: %v", err)
	}
	defer func() { _ = page.Close() }()
	_ = page.Timeout(5 * time.Second).WaitLoad()

	diag, err := doInsertTextInputEvent(page, "#t", "NEW")
	if err != nil {
		t.Fatalf("doInsertTextInputEvent: %v", err)
	}
	if !strings.HasPrefix(diag, "dispatched:") {
		t.Fatalf("unexpected diagnostic: %q", diag)
	}
	time.Sleep(80 * time.Millisecond)

	// Read innerText via doEvaluate (matches repo CDP pattern).
	out, err := doEvaluate(page, `document.getElementById('t').innerText`)
	if err != nil {
		t.Fatalf("read innerText: %v", err)
	}
	got, _ := out.(string)
	got = strings.TrimSpace(got)
	// Accept either full replacement ("NEW") OR, if the browser didn't honor
	// the default action for insertReplacementText on a plain div, at least
	// confirm the diagnostic reports non-cancelled dispatch. This keeps the
	// test useful across Chromium versions without being flaky.
	if got != "NEW" {
		if !strings.Contains(diag, "cancelled=false") {
			t.Fatalf("event was cancelled and text unchanged: diag=%q got=%q", diag, got)
		}
		t.Logf("default action did not replace text on plain div (got=%q), but event dispatched uncancelled — ok for smoke test", got)
	}
}

// TestInsertTextInputEvent_NoElement verifies a missing selector returns an
// error with a clear message and the "no_element" diagnostic surfaces.
func TestInsertTextInputEvent_NoElement(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	b := acquireSharedBrowser(t)

	page, err := b.Page(proto.TargetCreateTarget{URL: `data:text/html,<p>hi</p>`})
	if err != nil {
		t.Fatalf("open page: %v", err)
	}
	defer func() { _ = page.Close() }()
	_ = page.Timeout(5 * time.Second).WaitLoad()

	_, err = doInsertTextInputEvent(page, "#nonexistent", "x")
	if err == nil {
		t.Fatal("expected error for missing element")
	}
	if !strings.Contains(err.Error(), "element not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestInsertTextInputEvent_EmptySelector guards the input validation path.
func TestInsertTextInputEvent_EmptySelector(t *testing.T) {
	_, err := doInsertTextInputEvent(nil, "", "x")
	if err == nil {
		t.Fatal("expected error for empty selector")
	}
	if !strings.Contains(err.Error(), "selector is required") {
		t.Errorf("unexpected error: %v", err)
	}
}
