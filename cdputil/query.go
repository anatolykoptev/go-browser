package cdputil

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type selectorKind int

const (
	kindCSS selectorKind = iota
	kindText
	kindXPath
)

// NodeID is a CDP DOM node identifier.
type NodeID = proto.DOMNodeID

func parseSelector(raw string) (selectorKind, string) {
	switch {
	case strings.HasPrefix(raw, "text="):
		return kindText, strings.TrimPrefix(raw, "text=")
	case strings.HasPrefix(raw, "xpath="):
		return kindXPath, strings.TrimPrefix(raw, "xpath=")
	default:
		return kindCSS, raw
	}
}

// QuerySelector finds an element by selector using CDP DOM methods.
// Does NOT trigger Runtime.enable — safe for PX-protected pages.
func QuerySelector(page *rod.Page, selector string) (NodeID, error) {
	kind, sel := parseSelector(selector)
	switch kind {
	case kindCSS:
		return querySelectorCSS(page, sel)
	case kindText:
		return querySelectorText(page, sel)
	case kindXPath:
		return querySelectorXPath(page, sel)
	default:
		return 0, fmt.Errorf("unsupported selector kind")
	}
}

func querySelectorCSS(page *rod.Page, selector string) (NodeID, error) {
	depth := 0
	doc, err := (proto.DOMGetDocument{Depth: &depth}).Call(page)
	if err != nil {
		return 0, fmt.Errorf("DOM.getDocument: %w", err)
	}
	res, err := (proto.DOMQuerySelector{
		NodeID:   doc.Root.NodeID,
		Selector: selector,
	}).Call(page)
	if err != nil {
		return 0, fmt.Errorf("DOM.querySelector %q: %w", selector, err)
	}
	if res.NodeID == 0 {
		return 0, fmt.Errorf("element %q not found", selector)
	}
	return res.NodeID, nil
}

func querySelectorText(page *rod.Page, text string) (NodeID, error) {
	xpath := fmt.Sprintf(`//*[contains(text(), "%s")]`, strings.ReplaceAll(text, `"`, `\"`))
	return querySelectorXPath(page, xpath)
}

func querySelectorXPath(page *rod.Page, xpath string) (NodeID, error) {
	res, err := (proto.DOMPerformSearch{
		Query:                     xpath,
		IncludeUserAgentShadowDOM: false,
	}).Call(page)
	if err != nil {
		return 0, fmt.Errorf("DOM.performSearch %q: %w", xpath, err)
	}
	defer func() {
		_ = (proto.DOMDiscardSearchResults{SearchID: res.SearchID}).Call(page)
	}()
	if res.ResultCount == 0 {
		return 0, fmt.Errorf("element xpath=%q not found", xpath)
	}
	nodes, err := (proto.DOMGetSearchResults{
		SearchID:  res.SearchID,
		FromIndex: 0,
		ToIndex:   1,
	}).Call(page)
	if err != nil {
		return 0, fmt.Errorf("DOM.getSearchResults: %w", err)
	}
	if len(nodes.NodeIDs) == 0 {
		return 0, fmt.Errorf("element xpath=%q: no results", xpath)
	}
	return nodes.NodeIDs[0], nil
}
