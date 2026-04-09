package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/anatolykoptev/go-browser/cdputil"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// keyMap maps action key names to rod input keys.
//
//nolint:gochecknoglobals // static key mapping
var keyMap = map[string]input.Key{
	"Enter":      input.Enter,
	"Tab":        input.Tab,
	"Escape":     input.Escape,
	"Backspace":  input.Backspace,
	"Delete":     input.Delete,
	"ArrowUp":    input.ArrowUp,
	"ArrowDown":  input.ArrowDown,
	"ArrowLeft":  input.ArrowLeft,
	"ArrowRight": input.ArrowRight,
	"Space":      input.Space,
	"Home":       input.Home,
	"End":        input.End,
	"PageUp":     input.PageUp,
	"PageDown":   input.PageDown,
	"F1":         input.F1,
	"F2":         input.F2,
	"F3":         input.F3,
	"F4":         input.F4,
	"F5":         input.F5,
	"F6":         input.F6,
	"F7":         input.F7,
	"F8":         input.F8,
	"F9":         input.F9,
	"F10":        input.F10,
	"F11":        input.F11,
	"F12":        input.F12,
}

//nolint:gochecknoglobals // static modifier key mapping
var modifierKeyMap = map[string]input.Key{
	"Alt":     input.AltLeft,
	"Control": input.ControlLeft,
	"Shift":   input.ShiftLeft,
	"Meta":    input.MetaLeft,
}

// resolveElement finds an element using CSS, text=, or xpath= selector.
//
//nolint:cyclop // simple prefix dispatch
func resolveElement(ctx context.Context, page *rod.Page, selector string) (*rod.Element, error) {
	p := page.Context(ctx)
	switch {
	case strings.HasPrefix(selector, "text="):
		text := strings.TrimPrefix(selector, "text=")
		return p.ElementR("*", text)
	case strings.HasPrefix(selector, "xpath="):
		xpath := strings.TrimPrefix(selector, "xpath=")
		return p.ElementX(xpath)
	default:
		return p.Element(selector)
	}
}

func holdModifiers(page *rod.Page, modifiers []string) func() {
	var held []input.Key
	for _, m := range modifiers {
		if k, ok := modifierKeyMap[m]; ok {
			_ = page.Keyboard.Press(k)
			held = append(held, k)
		}
	}
	return func() {
		for _, k := range held {
			_ = page.Keyboard.Release(k)
		}
	}
}

func doClick(ctx context.Context, page *rod.Page, a Action) error {
	el, err := resolveElement(ctx, page, a.Selector)
	if err != nil {
		return fmt.Errorf("click: find %q: %w", a.Selector, err)
	}

	release := holdModifiers(page, a.Modifiers)
	defer release()

	btn := proto.InputMouseButtonLeft
	switch a.Button {
	case "right":
		btn = proto.InputMouseButtonRight
	case "middle":
		btn = proto.InputMouseButtonMiddle
	}

	clicks := 1
	if a.DoubleClick {
		clicks = 2
	}

	if err := el.Click(btn, clicks); err != nil {
		return fmt.Errorf("click: %w", err)
	}
	return nil
}

func doClickStealth(ctx context.Context, page *rod.Page, a Action) error {
	nodeID, err := cdputil.QuerySelector(page, a.Selector)
	if err != nil {
		return fmt.Errorf("click: %w", err)
	}
	_ = cdputil.ScrollIntoView(page, nodeID) // best-effort

	btn := proto.InputMouseButtonLeft
	switch a.Button {
	case "right":
		btn = proto.InputMouseButtonRight
	case "middle":
		btn = proto.InputMouseButtonMiddle
	}

	clicks := 1
	if a.DoubleClick {
		clicks = 2
	}

	return cdputil.ClickNode(page, nodeID, btn, clicks)
}
