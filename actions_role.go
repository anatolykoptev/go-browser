package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
)

// findByRole finds the first element matching the given ARIA role.
// It tries native HTML elements and [role=X] attributes in order.
func findByRole(ctx context.Context, page *rod.Page, role string) (*rod.Element, error) {
	p := page.Context(ctx)
	selectors := roleToSelectors(role)
	for _, sel := range selectors {
		el, err := p.Element(sel)
		if err == nil {
			return el, nil
		}
	}
	return nil, fmt.Errorf("role=%s: no matching element", role)
}

// roleToSelectors returns CSS selectors for a given ARIA role name.
// Native HTML elements are tried first, then explicit [role=X] attributes.
//
//nolint:cyclop // plain role mapping switch
func roleToSelectors(role string) []string {
	switch strings.ToLower(role) {
	case "button":
		return []string{"button", "[role=button]", "input[type=submit]", "input[type=button]"}
	case "link":
		return []string{"a[href]", "[role=link]"}
	case "textbox":
		return []string{
			"input[type=text]", "input[type=email]", "input[type=search]",
			"textarea", "[role=textbox]",
		}
	case "checkbox":
		return []string{"input[type=checkbox]", "[role=checkbox]"}
	case "radio":
		return []string{"input[type=radio]", "[role=radio]"}
	case "combobox":
		return []string{"select", "[role=combobox]", "[role=listbox]"}
	case "heading":
		return []string{"h1", "h2", "h3", "h4", "h5", "h6", "[role=heading]"}
	case "img", "image":
		return []string{"img", "[role=img]"}
	case "navigation":
		return []string{"nav", "[role=navigation]"}
	case "form":
		return []string{"form", "[role=form]"}
	default:
		return []string{"[role=" + strings.ToLower(role) + "]"}
	}
}
