package browser

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

// renderAXTree builds a plain text representation of the accessibility tree.
func renderAXTree(nodes []*proto.AccessibilityAXNode, maxDepth int) string {
	index, roots := buildAXIndex(nodes)

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

// buildAXIndex creates a node index and finds root nodes.
func buildAXIndex(nodes []*proto.AccessibilityAXNode) (map[string]*nodeInfo, []string) {
	index := make(map[string]*nodeInfo, len(nodes))
	isChild := make(map[string]bool, len(nodes))
	var allIDs []string

	for _, node := range nodes {
		if node.Ignored {
			continue
		}
		info := extractNodeInfo(node)
		if isNoiseRole(info.role) {
			continue
		}
		id := string(node.NodeID)
		for _, cid := range info.children {
			isChild[cid] = true
		}
		index[id] = info
		allIDs = append(allIDs, id)
	}

	var roots []string
	for _, id := range allIDs {
		if !isChild[id] {
			roots = append(roots, id)
		}
	}
	return index, roots
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

		// Collapse empty generic nodes — skip them, render children at same indent.
		if n.role == "generic" && n.name == "" && n.value == "" {
			for _, cid := range n.children {
				walk(cid, depth+1, indent)
			}
			return
		}

		prefix := strings.Repeat("  ", indent)
		line := formatYAMLNode(n, &refCounter)
		hasChildren := len(n.children) > 0
		hasDesc := n.description != ""

		if hasChildren || hasDesc {
			fmt.Fprintf(&sb, "%s- %s:\n", prefix, line)
		} else {
			fmt.Fprintf(&sb, "%s- %s\n", prefix, line)
		}

		if hasDesc {
			fmt.Fprintf(&sb, "%s  - /description: %s\n", prefix, n.description)
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

	// Ref for interactive roles.
	if interactiveRoles[n.role] {
		*refCounter++
		fmt.Fprintf(&sb, " [ref=e%d]", *refCounter)
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

	// Valued attributes.
	if n.level > 0 {
		fmt.Fprintf(&sb, " [level=%d]", n.level)
	}
	if n.value != "" {
		fmt.Fprintf(&sb, " [value=%q]", n.value)
	}

	return sb.String()
}
