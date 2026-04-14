package browser

import (
	"fmt"

	"github.com/go-rod/rod"
)

// doInsertTextInputEvent fills a contenteditable (TipTap/ProseMirror) by
// dispatching synthetic beforeinput + input events with inputType
// "insertReplacementText".
//
// Why this path exists:
//   - ProseMirror (used by TipTap/LinkedIn) intercepts keydown for Ctrl+A
//     through prosemirror-view, so doTypeTextCDP cannot clear the editor.
//   - execCommand('insertText') is undone by prosemirror-view's undo stack.
//   - InputEvent with inputType=insertReplacementText IS the modern spec
//     path browsers use for real keystrokes on contenteditable — ProseMirror
//     processes it through its proper transaction pipeline.
//
// The function:
//  1. Resolves the target element, scrolls into view, focuses it.
//  2. Places a Selection Range covering the full contents so the dispatched
//     event replaces existing text instead of appending.
//  3. Dispatches beforeinput then input with a DataTransfer payload.
//  4. Returns a diagnostic string like
//     "dispatched:cancelled=false|now=NEW HEADLINE..." so the caller can
//     verify mid-stream without a second evaluate action.
//
// For native <input>/<textarea>, prefer doTypeTextCDP — this function is
// specifically for Level-2 InputEvent-based rich editors.
func doInsertTextInputEvent(page *rod.Page, selector, text string) (string, error) {
	if selector == "" {
		return "", fmt.Errorf("insert_text_input_event: selector is required")
	}
	script := fmt.Sprintf(`
(function(){
  const el = document.querySelector(%q);
  if (!el) return 'no_element';
  try { el.scrollIntoView({block:'center'}); } catch (e) {}
  try { el.focus(); } catch (e) {}
  try {
    const r = document.createRange();
    r.selectNodeContents(el);
    const s = window.getSelection();
    s.removeAllRanges();
    s.addRange(r);
  } catch (e) { return 'range_err:' + (e && e.message); }

  const text = %q;
  let dt = null;
  try {
    dt = new DataTransfer();
    dt.setData('text/plain', text);
  } catch (e) { dt = null; }

  let before;
  try {
    before = new InputEvent('beforeinput', {
      inputType: 'insertReplacementText',
      data: text,
      dataTransfer: dt,
      bubbles: true,
      cancelable: true,
      composed: true
    });
  } catch (e) { return 'ctor_err:' + (e && e.message); }

  const ok = el.dispatchEvent(before);
  if (ok) {
    try {
      const after = new InputEvent('input', {
        inputType: 'insertReplacementText',
        data: text,
        bubbles: true,
        composed: true
      });
      el.dispatchEvent(after);
    } catch (e) {}
  }
  const now = (el.innerText || el.textContent || '').slice(0, 120);
  return 'dispatched:cancelled=' + (!ok) + '|now=' + now;
})()`, selector, text)

	res, err := doEvaluate(page, script)
	if err != nil {
		return "", fmt.Errorf("insert_text_input_event: %w", err)
	}
	s, _ := res.(string)
	if s == "no_element" {
		return s, fmt.Errorf("insert_text_input_event: element not found: %s", selector)
	}
	return s, nil
}
