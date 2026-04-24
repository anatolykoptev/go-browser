package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

// resolveElement finds an element using ref=, CSS, text=, xpath=, or role= selector.
//
//nolint:cyclop // simple prefix dispatch
func resolveElement(ctx context.Context, page *rod.Page, selector string, refMap *RefMap) (*rod.Element, error) {
	// Ref-based resolution: ref=eN → BackendDOMNodeID → rod.Element.
	if ref, ok := ParseRef(selector); ok {
		if refMap == nil {
			return nil, fmt.Errorf("ref %q used but no snapshot taken yet", ref)
		}
		backendID, found := refMap.Resolve(ref)
		if !found {
			return nil, fmt.Errorf("ref %q not found — take a new snapshot", ref)
		}
		// Resolve BackendNodeID → RemoteObjectID via CDP DOM.resolveNode.
		resolved, err := proto.DOMResolveNode{BackendNodeID: backendID}.Call(page)
		if err != nil {
			return nil, fmt.Errorf("ref %q: resolve node: %w", ref, err)
		}
		el, elErr := page.Context(ctx).ElementFromObject(resolved.Object)
		if elErr != nil {
			return nil, fmt.Errorf("ref %q: element from object: %w", ref, elErr)
		}
		return el, nil
	}

	p := page.Context(ctx)
	switch {
	case strings.HasPrefix(selector, "text="):
		text := strings.TrimPrefix(selector, "text=")
		// Try rod's built-in regex text selector first.
		el, err := p.ElementR("*", text)
		if err == nil {
			return el, nil
		}
		// Fallback: XPath search for clickable ancestor containing the text.
		el, err = findByText(ctx, page, text)
		if err != nil {
			return nil, fmt.Errorf("text=%s: %w", text, err)
		}
		return el, nil
	case strings.HasPrefix(selector, "xpath="):
		xpath := strings.TrimPrefix(selector, "xpath=")
		return p.ElementX(xpath)
	case strings.HasPrefix(selector, "role="):
		role := strings.TrimPrefix(selector, "role=")
		return findByRole(ctx, page, role)
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

// resolveRefNodeID resolves a ref= selector to a CDP NodeID, or falls back to cdputil.QuerySelector.
func resolveRefNodeID(page *rod.Page, selector string, refMap *RefMap) (cdputil.NodeID, error) {
	if ref, ok := ParseRef(selector); ok {
		if refMap == nil {
			return 0, fmt.Errorf("ref %q used but no snapshot taken yet", ref)
		}
		backendID, found := refMap.Resolve(ref)
		if !found {
			return 0, fmt.Errorf("ref %q not found — take a new snapshot", ref)
		}
		res, err := proto.DOMDescribeNode{BackendNodeID: backendID}.Call(page)
		if err != nil {
			return 0, fmt.Errorf("ref %q: describe node: %w", ref, err)
		}
		return res.Node.NodeID, nil
	}
	return cdputil.QuerySelector(page, selector)
}

// clickDeadline caps a click when the caller supplied neither action.TimeoutMs
// nor an overall chain deadline short enough to bite. rod's WaitInteractable
// retries on CoveredError until ctx cancels; without this cap a covered
// element silently consumes the full interact budget (default 30s). 5s is
// long enough for a normal scroll-into-view + overlay dismissal animation
// but short enough to fail-fast on genuinely stuck overlays.
const clickDeadline = 5 * time.Second

func doClick(ctx context.Context, page *rod.Page, a Action, refMap *RefMap) error {
	el, err := resolveElement(ctx, page, a.Selector, refMap)
	if err != nil {
		return fmt.Errorf("click: find %q: %w", a.Selector, errors.Join(err, ErrSelectorNotFound))
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

	// Bound the click — see clickDeadline docstring.
	clickCtx, cancel := context.WithTimeout(ctx, clickDeadline)
	defer cancel()
	el = el.Context(clickCtx)

	// Probe interactability up-front so we can surface a typed CoveredError
	// instead of the opaque "deadline exceeded" rod returns when retry spins.
	// A single Interactable() call does not retry/scroll; it just reports the
	// current state. rod's Click → Hover still handles scroll + retry, we just
	// capture the last covered state for the relabel step below.
	var lastCovered *rod.CoveredError
	if _, interactErr := el.Interactable(); interactErr != nil {
		var covered *rod.CoveredError
		if errors.As(interactErr, &covered) {
			lastCovered = covered
		}
	}

	clickErr := el.Click(btn, clicks)
	if clickErr == nil {
		return nil
	}

	// Relabel CoveredError / deadline-hit-while-covered as element_covered
	// so agents can distinguish "someone is covering this" from "element
	// doesn't exist / is off-screen". Before this, both paths came out as
	// InvisibleShapeError + naked deadline text.
	var covered *rod.CoveredError
	if errors.As(clickErr, &covered) {
		return fmt.Errorf("click: %s: %w", covered.Error(), ErrElementCovered)
	}
	// Deadline while retrying on CoveredError: rod returns ctx.Err() directly,
	// losing the covered-element identity. Use the up-front probe result.
	if errors.Is(clickErr, context.DeadlineExceeded) && lastCovered != nil {
		return fmt.Errorf("click: %s (timed out waiting for overlay to clear after %s): %w",
			lastCovered.Error(), clickDeadline, ErrElementCovered)
	}
	if errors.Is(clickErr, context.DeadlineExceeded) {
		return fmt.Errorf("click: timed out after %s: %w", clickDeadline, ErrActionTimeout)
	}
	return fmt.Errorf("click: %w", clickErr)
}

func doClickStealth(ctx context.Context, page *rod.Page, a Action, refMap *RefMap) error {
	nodeID, err := resolveRefNodeID(page, a.Selector, refMap)
	if err != nil {
		return fmt.Errorf("click: %w", errors.Join(err, ErrSelectorNotFound))
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
