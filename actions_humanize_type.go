package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anatolykoptev/go-browser/humanize"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// doTypeTextHumanized types text character by character with human-like delays,
// dispatching real keydown/char/keyup CDP events so keyboard listeners see authentic input.
func doTypeTextHumanized(
	ctx context.Context, page *rod.Page, selector, text string, cursor *humanize.Cursor,
) error {
	if err := doClickHumanized(ctx, page, selector, cursor); err != nil {
		return fmt.Errorf("type_text: focus: %w", err)
	}

	delays := humanize.TypingDelays(text)
	for i, ch := range text {
		char := string(ch)
		code := charToCode(ch)
		vk := charToVK(ch)

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeRawKeyDown,
			Key:                   char,
			Code:                  code,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeChar,
			Text:                  char,
			UnmodifiedText:        char,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		_ = proto.InputDispatchKeyEvent{
			Type:                  proto.InputDispatchKeyEventTypeKeyUp,
			Key:                   char,
			Code:                  code,
			WindowsVirtualKeyCode: vk,
		}.Call(page)

		if i < len(delays) {
			sleepCtx(ctx, time.Duration(delays[i])*time.Millisecond)
		}
	}
	return nil
}

// charToVK maps a character to its Windows Virtual Key code.
func charToVK(ch rune) int {
	switch {
	case ch >= 'a' && ch <= 'z':
		return int(ch - 32) // VK_A=65 .. VK_Z=90
	case ch >= 'A' && ch <= 'Z':
		return int(ch)
	case ch >= '0' && ch <= '9':
		return int(ch) // VK_0=48 .. VK_9=57
	case ch == ' ':
		return 32 // VK_SPACE
	case ch == '.':
		return 190 // VK_OEM_PERIOD
	case ch == ',':
		return 188 // VK_OEM_COMMA
	case ch == '-':
		return 189 // VK_OEM_MINUS
	case ch == '=':
		return 187 // VK_OEM_PLUS
	case ch == '@':
		return 50 // Shift+2
	case ch == '_':
		return 189 // Shift+Minus
	case ch == '!':
		return 49 // Shift+1
	case ch == '/':
		return 191 // VK_OEM_2
	case ch == ':':
		return 186 // Shift+;
	case ch == ';':
		return 186 // VK_OEM_1
	default:
		return int(ch)
	}
}

// charToCode maps a character to its DOM KeyboardEvent.code value.
func charToCode(ch rune) string {
	switch {
	case ch >= 'a' && ch <= 'z':
		return "Key" + strings.ToUpper(string(ch))
	case ch >= 'A' && ch <= 'Z':
		return "Key" + string(ch)
	case ch >= '0' && ch <= '9':
		return "Digit" + string(ch)
	case ch == ' ':
		return "Space"
	case ch == '.':
		return "Period"
	case ch == ',':
		return "Comma"
	case ch == '-':
		return "Minus"
	case ch == '=':
		return "Equal"
	case ch == '@':
		return "Digit2"
	case ch == '_':
		return "Minus"
	case ch == '!':
		return "Digit1"
	default:
		return ""
	}
}
