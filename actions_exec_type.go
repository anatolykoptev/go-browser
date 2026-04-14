package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/anatolykoptev/go-browser/cdputil"
	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

func doTypeText(ctx context.Context, page *rod.Page, selector, text string, slowly, submit bool, refMap *RefMap) error {
	if slowly {
		// CDP char-by-char path — for bot-detection protected pages (LinkedIn, etc.).
		// Uses JS focus + CDP dispatchKeyEvent which triggers React onChange
		// and bypasses PX/bot-detection event interception.
		return doTypeTextCDP(ctx, page, selector, text, submit, refMap)
	}

	// Default fast path — rod's Input() via Runtime.callFunctionOn.
	// Works for most sites. Falls back to CDP path on timeout.
	el, err := resolveElement(ctx, page, selector, refMap)
	if err != nil {
		return fmt.Errorf("type_text: find %q: %w", selector, err)
	}
	_ = el.SelectAllText()
	if err := el.Input(text); err != nil {
		return fmt.Errorf("type_text: input: %w", err)
	}
	if submit {
		if err := page.Keyboard.Press(input.Enter); err != nil {
			return fmt.Errorf("type_text: submit: %w", err)
		}
	}
	return nil
}

// doTypeTextCDP types text using pure CDP events — reliable on PX-protected pages.
// Focus via CDP DOM.focus (no Runtime.callFunctionOn), clear via Ctrl+A+Delete, type via dispatchKeyEvent.
func doTypeTextCDP(ctx context.Context, page *rod.Page, selector, text string, submit bool, refMap *RefMap) error {
	nodeID, err := resolveRefNodeID(page, selector, refMap)
	if err != nil {
		return fmt.Errorf("type_text: %w", err)
	}
	if err := cdputil.FocusNode(page, nodeID); err != nil {
		return fmt.Errorf("type_text: focus: %w", err)
	}

	// Clear via Ctrl+A then Delete.
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: "a", Code: "KeyA",
		WindowsVirtualKeyCode: 65, Modifiers: 2,
	}).Call(page)
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeKeyUp, Key: "a", Code: "KeyA",
	}).Call(page)
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: "Delete", Code: "Delete",
		WindowsVirtualKeyCode: 46,
	}).Call(page)
	_ = (proto.InputDispatchKeyEvent{
		Type: proto.InputDispatchKeyEventTypeKeyUp, Key: "Delete", Code: "Delete",
	}).Call(page)

	for _, ch := range text {
		char := string(ch)
		ci := humanize.LookupChar(ch)
		code := ci.Code
		vk := ci.VK

		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: char, Code: code,
			WindowsVirtualKeyCode: vk,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeChar, Text: char, UnmodifiedText: char,
			WindowsVirtualKeyCode: vk,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeKeyUp, Key: char, Code: code,
			WindowsVirtualKeyCode: vk,
		}).Call(page)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}

	if submit {
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeRawKeyDown, Key: "Enter", Code: "Enter",
			WindowsVirtualKeyCode: 13,
		}).Call(page)
		_ = (proto.InputDispatchKeyEvent{
			Type: proto.InputDispatchKeyEventTypeKeyUp, Key: "Enter", Code: "Enter",
			WindowsVirtualKeyCode: 13,
		}).Call(page)
	}
	return nil
}

func doFillForm(ctx context.Context, page *rod.Page, fields []FormField, refMap *RefMap) error {
	for _, f := range fields {
		el, err := resolveElement(ctx, page, f.Selector, refMap)
		if err != nil {
			return fmt.Errorf("fill_form: find %q: %w", f.Selector, err)
		}
		switch f.Type {
		case "checkbox":
			checked, _ := el.Property("checked")
			want := f.Value == "true"
			if checked.Bool() != want {
				if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
					return fmt.Errorf("fill_form: checkbox %q: %w", f.Selector, err)
				}
			}
		case "combobox":
			if err := el.Select([]string{f.Value}, true, rod.SelectorTypeText); err != nil {
				return fmt.Errorf("fill_form: select %q: %w", f.Selector, err)
			}
		default: // textbox
			_ = el.SelectAllText()
			if err := el.Input(f.Value); err != nil {
				return fmt.Errorf("fill_form: input %q: %w", f.Selector, err)
			}
		}
	}
	return nil
}

func doFillFormStealth(ctx context.Context, page *rod.Page, fields []FormField, refMap *RefMap) error {
	for _, f := range fields {
		switch f.Type {
		case "checkbox":
			nodeID, err := resolveRefNodeID(page, f.Selector, refMap)
			if err != nil {
				return fmt.Errorf("fill_form: find %q: %w", f.Selector, err)
			}
			if err := cdputil.ClickNode(page, nodeID, proto.InputMouseButtonLeft, 1); err != nil {
				return fmt.Errorf("fill_form: checkbox %q: %w", f.Selector, err)
			}
		default:
			if err := doTypeTextCDP(ctx, page, f.Selector, f.Value, false, refMap); err != nil {
				return fmt.Errorf("fill_form: %w", err)
			}
		}
	}
	return nil
}

// doSelectAll selects all text using JavaScript Selection API.
// This bypasses TipTap/ProseMirror keyboard interceptors that block Ctrl+A.
// If selector is empty, uses the currently focused element.
func doSelectAll(page *rod.Page, selector string, refMap *RefMap) error {
	var script string
	if selector != "" {
		// Select all in specific element
		script = fmt.Sprintf(`
			(function() {
				const el = document.querySelector(%q);
				if (!el) return false;
				el.focus();
				const range = document.createRange();
				range.selectNodeContents(el);
				const sel = window.getSelection();
				sel.removeAllRanges();
				sel.addRange(range);
				return true;
			})()
		`, selector)
	} else {
		// Select all in focused element or document
		script = `
			(function() {
				const sel = window.getSelection();
				const el = document.activeElement || document.body;
				if (el && el.select) {
					// Standard input/textarea
					el.select();
					return true;
				}
				// ContentEditable or complex editors
				if (el) {
					const range = document.createRange();
					range.selectNodeContents(el);
					sel.removeAllRanges();
					sel.addRange(range);
					return true;
				}
				// Fallback: select all document
				document.execCommand('selectAll');
				return true;
			})()
		`
	}
	_, err := doEvaluate(page, script)
	if err != nil {
		return fmt.Errorf("select_all: %w", err)
	}
	return nil
}
