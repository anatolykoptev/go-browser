package browser

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// nodeInfo holds the extracted accessibility properties of a single AX node.
type nodeInfo struct {
	role, name, value, description       string
	children                             []string
	focused, disabled, checked, expanded bool
	selected, required, readonly         bool
	level                                int
	hasPopup                             string
	invalid                              string
	autoComplete                         string
}

// extractNodeInfo builds a nodeInfo from a CDP AccessibilityAXNode.
func extractNodeInfo(node *proto.AccessibilityAXNode) *nodeInfo {
	info := &nodeInfo{}

	if node.Role != nil {
		info.role = fmt.Sprintf("%v", node.Role.Value.Val())
	}
	if node.Name != nil {
		info.name = fmt.Sprintf("%v", node.Name.Value.Val())
	}
	if node.Value != nil {
		info.value = fmt.Sprintf("%v", node.Value.Value.Val())
	}
	if node.Description != nil {
		info.description = fmt.Sprintf("%v", node.Description.Value.Val())
	}

	info.children = make([]string, 0, len(node.ChildIDs))
	for _, cid := range node.ChildIDs {
		info.children = append(info.children, string(cid))
	}

	for _, prop := range node.Properties {
		v := prop.Value
		switch prop.Name {
		case "focused":
			info.focused = toBool(v)
		case "disabled":
			info.disabled = toBool(v)
		case "checked":
			info.checked = toBool(v)
		case "expanded":
			info.expanded = toBool(v)
		case "selected":
			info.selected = toBool(v)
		case "required":
			info.required = toBool(v)
		case "readonly":
			info.readonly = toBool(v)
		case "level":
			if v != nil {
				if f, ok := v.Value.Val().(float64); ok {
					info.level = int(f)
				}
			}
		case "haspopup":
			if v != nil {
				info.hasPopup = fmt.Sprintf("%v", v.Value.Val())
			}
		case "invalid":
			if v != nil {
				info.invalid = fmt.Sprintf("%v", v.Value.Val())
			}
		case "autocomplete":
			if v != nil {
				info.autoComplete = fmt.Sprintf("%v", v.Value.Val())
			}
		}
	}

	return info
}

// toBool extracts a boolean value from an AXValue.
func toBool(v *proto.AccessibilityAXValue) bool {
	if v == nil {
		return false
	}
	b, ok := v.Value.Val().(bool)
	return ok && b
}

// isNoiseRole returns true for roles that add no meaningful content to the tree.
func isNoiseRole(role string) bool {
	switch role {
	case "StaticText", "InlineTextBox", "none", "LineBreak":
		return true
	default:
		return false
	}
}

func doSnapshot(page *rod.Page, maxDepth int, format string) (string, error) {
	// Collect AX trees from main frame + all child frames.
	allNodes := collectAXNodes(page, proto.PageFrameID(""))

	// Also collect from child iframes via FrameTree.
	frames, err := proto.PageGetFrameTree{}.Call(page)
	if err == nil && frames.FrameTree != nil {
		walkFrameTree(page, frames.FrameTree, &allNodes)
	}

	// Fallback: if FrameTree found no child frames, try via TargetGetTargets
	// to discover OOP (out-of-process) iframes.
	if err != nil || (frames.FrameTree != nil && len(frames.FrameTree.ChildFrames) == 0) {
		targets, terr := proto.TargetGetTargets{}.Call(page)
		if terr == nil {
			for _, t := range targets.TargetInfos {
				if t.Type == "iframe" {
					childNodes := collectAXNodes(page, proto.PageFrameID(t.TargetID))
					allNodes = append(allNodes, childNodes...)
				}
			}
		}
	}

	tree := renderAXTree(allNodes, maxDepth)

	// Debug: if tree is suspiciously short, append node stats.
	if len(allNodes) > 0 && len(tree) < 200 {
		ignored := 0
		for _, n := range allNodes {
			if n.Ignored {
				ignored++
			}
		}
		tree += fmt.Sprintf("\n<!-- ax_debug: total=%d ignored=%d rendered=%d frames_err=%v -->",
			len(allNodes), ignored, len(allNodes)-ignored, err)
	}

	_ = format // reserved for future YAML/JSON output modes
	return tree, nil
}

// walkFrameTree recursively visits all child frames and appends their AX nodes.
func walkFrameTree(page *rod.Page, tree *proto.PageFrameTree, allNodes *[]*proto.AccessibilityAXNode) {
	for _, child := range tree.ChildFrames {
		childNodes := collectAXNodes(page, proto.PageFrameID(child.Frame.ID))
		*allNodes = append(*allNodes, childNodes...)
		walkFrameTree(page, child, allNodes)
	}
}

// collectAXNodes fetches the accessibility tree for a single frame.
func collectAXNodes(page *rod.Page, frameID proto.PageFrameID) []*proto.AccessibilityAXNode {
	req := proto.AccessibilityGetFullAXTree{}
	if frameID != "" {
		req.FrameID = frameID
	}
	res, err := req.Call(page)
	if err != nil {
		return nil
	}
	return res.Nodes
}

// renderAXTree builds a text representation of the accessibility tree.
func renderAXTree(nodes []*proto.AccessibilityAXNode, maxDepth int) string {
	index := make(map[string]*nodeInfo, len(nodes))
	// Track all IDs that appear as children — non-child IDs are roots.
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

	// Find all root nodes (nodes that are not children of any other node).
	var roots []string
	for _, id := range allIDs {
		if !isChild[id] {
			roots = append(roots, id)
		}
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
