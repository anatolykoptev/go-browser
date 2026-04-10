package browser

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// renderAXTreeYAMLWithURLsAndRefs builds a YAML snapshot with link URL extraction
// and populates rm with ref → BackendDOMNodeID mappings (when rm is non-nil).
func renderAXTreeYAMLWithURLsAndRefs(
	nodes []*proto.AccessibilityAXNode, maxDepth int, page *rod.Page, filter, selector string, rm *RefMap,
) string {
	index, roots := buildAXIndex(nodes)
	urls := collectLinkURLs(page)
	if urls != nil {
		applyLinkURLs(index, nodes, urls)
	}
	if filter != "" || selector != "" {
		index, roots = filterAXTree(index, roots, filter, selector)
	}
	if rm != nil {
		return renderYAMLWithRefs(index, roots, maxDepth, rm)
	}
	return renderYAML(index, roots, maxDepth)
}

// renderAXTree builds a plain text representation of the accessibility tree.
func renderAXTree(nodes []*proto.AccessibilityAXNode, maxDepth int, filter, selector string) string {
	index, roots := buildAXIndex(nodes)
	if filter != "" || selector != "" {
		index, roots = filterAXTree(index, roots, filter, selector)
	}

	var sb strings.Builder
	var walk func(id string, level int)
	walk = func(id string, level int) {
		if maxDepth > 0 && level >= maxDepth {
			return
		}
		n, ok := index[id]
		if !ok {
			return
		}
		if n.role != "" || n.name != "" {
			indent := strings.Repeat("  ", level)
			fmt.Fprintf(&sb, "%s[%s] %s\n", indent, n.role, n.name)
		}
		for _, cid := range n.children {
			walk(cid, level+1)
		}
	}
	for _, rootID := range roots {
		walk(rootID, 0)
	}
	return sb.String()
}

// interactiveRoles defines roles that receive [ref=eN] numbering.
var interactiveRoles = map[string]bool{
	"link": true, "button": true, "textbox": true, "combobox": true,
	"checkbox": true, "radio": true, "slider": true, "switch": true,
	"tab": true, "menuitem": true, "option": true, "searchbox": true,
	"spinbutton": true, "treeitem": true, "menuitemcheckbox": true,
	"menuitemradio": true, "listbox": true,
}

// renderAXTreeYAML builds a Playwright-compatible YAML representation of the
// accessibility tree with ref numbering for interactive elements.
func renderAXTreeYAML(nodes []*proto.AccessibilityAXNode, maxDepth int) string {
	index, roots := buildAXIndex(nodes)
	return renderYAML(index, roots, maxDepth)
}

// renderYAML builds the YAML output from a pre-built index and root list.
func renderYAML(index map[string]*nodeInfo, roots []string, maxDepth int) string {
	var sb strings.Builder
	refCounter := 0

	var walk func(id string, depth, indent int)
	walk = func(id string, depth, indent int) {
		if maxDepth > 0 && depth >= maxDepth {
			return
		}
		n, ok := index[id]
		if !ok {
			return
		}

		// Skip empty leaf generic nodes (no name, no text, no visible children).
		if n.role == "generic" && n.name == "" && n.value == "" && n.text == "" {
			if !hasVisibleChildren(n, index) {
				return
			}
			// Generic wrapper with children but no content: skip line, render children.
			for _, cid := range n.children {
				walk(cid, depth+1, indent)
			}
			return
		}

		prefix := strings.Repeat("  ", indent)
		line := formatYAMLNode(n, &refCounter)
		hasChildren := hasVisibleChildren(n, index)
		hasDesc := n.description != ""
		hasURL := n.url != ""

		// Inline text: if node has text and no children, append after colon.
		if n.text != "" && !hasChildren && !hasDesc && !hasURL {
			fmt.Fprintf(&sb, "%s- %s: %s\n", prefix, line, n.text)
		} else if hasChildren || hasDesc || hasURL || n.text != "" {
			fmt.Fprintf(&sb, "%s- %s:\n", prefix, line)
			if n.text != "" {
				fmt.Fprintf(&sb, "%s  - text: %s\n", prefix, n.text)
			}
		} else {
			fmt.Fprintf(&sb, "%s- %s\n", prefix, line)
		}

		if hasDesc {
			fmt.Fprintf(&sb, "%s  - /description: %s\n", prefix, n.description)
		}
		if hasURL {
			fmt.Fprintf(&sb, "%s  - /url: %s\n", prefix, n.url)
		}

		for _, cid := range n.children {
			walk(cid, depth+1, indent+1)
		}
	}

	for _, rootID := range roots {
		walk(rootID, 0, 0)
	}

	return sb.String()
}

// renderYAMLWithRefs builds YAML output and populates rm with ref → BackendDOMNodeID mappings.
func renderYAMLWithRefs(index map[string]*nodeInfo, roots []string, maxDepth int, rm *RefMap) string {
	rm.Clear()
	var sb strings.Builder
	refCounter := 0

	var walk func(id string, depth, indent int)
	walk = func(id string, depth, indent int) {
		if maxDepth > 0 && depth >= maxDepth {
			return
		}
		n, ok := index[id]
		if !ok {
			return
		}
		if n.role == "generic" && n.name == "" && n.value == "" && n.text == "" {
			if !hasVisibleChildren(n, index) {
				return
			}
			for _, cid := range n.children {
				walk(cid, depth+1, indent)
			}
			return
		}

		prefix := strings.Repeat("  ", indent)
		oldCounter := refCounter
		line := formatYAMLNode(n, &refCounter)

		// If a new ref was assigned, store the mapping.
		if refCounter > oldCounter && n.backendNodeID != 0 {
			ref := fmt.Sprintf("e%d", refCounter)
			rm.Store(ref, n.backendNodeID)
		}

		hasChildren := hasVisibleChildren(n, index)
		hasDesc := n.description != ""
		hasURL := n.url != ""

		if n.text != "" && !hasChildren && !hasDesc && !hasURL {
			fmt.Fprintf(&sb, "%s- %s: %s\n", prefix, line, n.text)
		} else if hasChildren || hasDesc || hasURL || n.text != "" {
			fmt.Fprintf(&sb, "%s- %s:\n", prefix, line)
			if n.text != "" {
				fmt.Fprintf(&sb, "%s  - text: %s\n", prefix, n.text)
			}
		} else {
			fmt.Fprintf(&sb, "%s- %s\n", prefix, line)
		}
		if hasDesc {
			fmt.Fprintf(&sb, "%s  - /description: %s\n", prefix, n.description)
		}
		if hasURL {
			fmt.Fprintf(&sb, "%s  - /url: %s\n", prefix, n.url)
		}
		for _, cid := range n.children {
			walk(cid, depth+1, indent+1)
		}
	}

	for _, rootID := range roots {
		walk(rootID, 0, 0)
	}
	return sb.String()
}

// formatYAMLNode formats a single node as: role "name" [attrs...]
func formatYAMLNode(n *nodeInfo, refCounter *int) string {
	var sb strings.Builder
	sb.WriteString(n.role)

	if n.name != "" {
		fmt.Fprintf(&sb, " %q", n.name)
	}

	// Ref and cursor for interactive roles.
	if interactiveRoles[n.role] {
		*refCounter++
		fmt.Fprintf(&sb, " [ref=e%d] [cursor=pointer]", *refCounter)
	}

	// Boolean attributes.
	if n.focused {
		sb.WriteString(" [focused]")
	}
	if n.disabled {
		sb.WriteString(" [disabled]")
	}
	if n.checked {
		sb.WriteString(" [checked]")
	}
	if n.expanded {
		sb.WriteString(" [expanded]")
	}
	if n.selected {
		sb.WriteString(" [selected]")
	}
	if n.required {
		sb.WriteString(" [required]")
	}
	if n.readonly {
		sb.WriteString(" [readonly]")
	}
	if n.hidden {
		sb.WriteString(" [hidden]")
	}
	if n.modal {
		sb.WriteString(" [modal]")
	}
	if n.live != "" {
		fmt.Fprintf(&sb, " [live=%s]", n.live)
	}

	// Valued attributes.
	if n.level > 0 {
		fmt.Fprintf(&sb, " [level=%d]", n.level)
	}
	if n.value != "" {
		fmt.Fprintf(&sb, " [value=%q]", n.value)
	}

	return sb.String()
}
