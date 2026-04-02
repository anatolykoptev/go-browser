package browser

import (
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

// buildAXIndex creates a node index and finds root nodes.
// Pass 1: build all nodes (including noise roles).
// Pass 2: collect text from noise children onto parents, then remove noise nodes.
func buildAXIndex(nodes []*proto.AccessibilityAXNode) (map[string]*nodeInfo, []string) {
	all := make(map[string]*nodeInfo, len(nodes))
	isChild := make(map[string]bool, len(nodes))
	var allIDs []string

	// Pass 1: index every non-ignored node.
	for _, node := range nodes {
		if node.Ignored {
			continue
		}
		info := extractNodeInfo(node)
		id := string(node.NodeID)
		for _, cid := range info.children {
			isChild[cid] = true
		}
		all[id] = info
		allIDs = append(allIDs, id)
	}

	// Pass 2: propagate text from noise children to parent, then prune.
	for _, id := range allIDs {
		n := all[id]
		if isNoiseRole(n.role) {
			continue
		}
		var kept []string
		var texts []string
		for _, cid := range n.children {
			child, ok := all[cid]
			if !ok {
				continue
			}
			if isNoiseRole(child.role) {
				if child.name != "" {
					texts = append(texts, child.name)
				}
			} else {
				kept = append(kept, cid)
			}
		}
		if len(texts) > 0 && n.text == "" {
			n.text = strings.Join(texts, " ")
		}
		n.children = kept
	}

	// Build final index without noise nodes.
	index := make(map[string]*nodeInfo, len(allIDs))
	var cleanIDs []string
	for _, id := range allIDs {
		if !isNoiseRole(all[id].role) {
			index[id] = all[id]
			cleanIDs = append(cleanIDs, id)
		}
	}

	var roots []string
	for _, id := range cleanIDs {
		if !isChild[id] {
			roots = append(roots, id)
		}
	}
	return index, roots
}

// hasVisibleChildren returns true if the node has at least one child in the index.
func hasVisibleChildren(n *nodeInfo, index map[string]*nodeInfo) bool {
	for _, cid := range n.children {
		if _, ok := index[cid]; ok {
			return true
		}
	}
	return false
}
