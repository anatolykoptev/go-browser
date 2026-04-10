package browser

import "strings"

// filterableRoles extends interactiveRoles with structural/grouping roles kept by "interactive" mode.
var filterableRoles = func() map[string]bool {
	m := make(map[string]bool, len(interactiveRoles)+8)
	for k, v := range interactiveRoles {
		m[k] = v
	}
	extras := []string{"form", "radiogroup", "group", "list", "listitem", "region", "LabelText", "Iframe"}
	for _, r := range extras {
		m[r] = true
	}
	return m
}()

// excludeTextRoles are excluded in "text" filter mode.
var excludeTextRoles = map[string]bool{
	"navigation": true, "banner": true, "contentinfo": true,
}

// filterAXTree returns a filtered copy of the index and roots according to filter/selector.
// filter values: "" (no-op), "interactive", "forms", "main", "text".
// selector: when non-empty, keep only nodes whose name/role/url contains it (case-insensitive).
func filterAXTree(
	index map[string]*nodeInfo,
	roots []string,
	filter, selector string,
) (map[string]*nodeInfo, []string) {
	if filter == "" && selector == "" {
		return index, roots
	}

	keep := make(map[string]bool, len(index))

	switch filter {
	case "interactive":
		applyInteractiveFilter(index, roots, keep)
	case "forms":
		applySubtreeFilter(index, roots, "form", keep)
	case "main":
		applySubtreeFilter(index, roots, "main", keep)
	case "text":
		applyTextFilter(index, roots, keep)
	default:
		// Unknown filter — keep everything, apply selector only.
		for id := range index {
			keep[id] = true
		}
	}

	// Apply selector narrowing on top of whatever the filter kept.
	if selector != "" {
		sel := strings.ToLower(selector)
		for id := range keep {
			n := index[id]
			if !strings.Contains(strings.ToLower(n.name), sel) &&
				!strings.Contains(strings.ToLower(n.role), sel) &&
				!strings.Contains(strings.ToLower(n.url), sel) {
				delete(keep, id)
			}
		}
	}

	return buildFilteredIndex(index, roots, keep)
}

// applyInteractiveFilter marks filterable roles + their ancestors.
func applyInteractiveFilter(index map[string]*nodeInfo, roots []string, keep map[string]bool) {
	// Build parent map for ancestor traversal.
	parent := buildParentMap(index, roots)

	for id, n := range index {
		if filterableRoles[n.role] {
			markWithAncestors(id, index, parent, keep)
		}
	}
}

// applySubtreeFilter marks all subtrees rooted at nodes with the given role.
func applySubtreeFilter(index map[string]*nodeInfo, roots []string, role string, keep map[string]bool) {
	_ = roots
	for id, n := range index {
		if n.role == role {
			markSubtree(id, index, keep)
		}
	}
}

// applyTextFilter keeps everything except navigation/banner/contentinfo subtrees.
func applyTextFilter(index map[string]*nodeInfo, roots []string, keep map[string]bool) {
	for id, n := range index {
		if excludeTextRoles[n.role] {
			continue
		}
		keep[id] = true
	}
	// Remove children that are inside an excluded subtree.
	excluded := make(map[string]bool)
	for id, n := range index {
		if excludeTextRoles[n.role] {
			markSubtree(id, index, excluded)
		}
	}
	for id := range excluded {
		delete(keep, id)
	}
	_ = roots
}

// markWithAncestors marks a node and all of its ancestors.
func markWithAncestors(id string, index map[string]*nodeInfo, parent map[string]string, keep map[string]bool) {
	cur := id
	for cur != "" {
		if keep[cur] {
			break // already processed this branch
		}
		if _, ok := index[cur]; !ok {
			break
		}
		keep[cur] = true
		cur = parent[cur]
	}
}

// markSubtree marks a node and all of its descendants.
func markSubtree(id string, index map[string]*nodeInfo, keep map[string]bool) {
	n, ok := index[id]
	if !ok {
		return
	}
	keep[id] = true
	for _, cid := range n.children {
		markSubtree(cid, index, keep)
	}
}

// buildParentMap builds a child→parent id map by walking the tree from roots.
func buildParentMap(index map[string]*nodeInfo, roots []string) map[string]string {
	parent := make(map[string]string, len(index))
	var walk func(id, par string)
	walk = func(id, par string) {
		parent[id] = par
		n, ok := index[id]
		if !ok {
			return
		}
		for _, cid := range n.children {
			walk(cid, id)
		}
	}
	for _, r := range roots {
		walk(r, "")
	}
	return parent
}

// buildFilteredIndex constructs a new index containing only nodes in keep.
// Children lists are pruned to only include kept nodes.
func buildFilteredIndex(
	index map[string]*nodeInfo,
	roots []string,
	keep map[string]bool,
) (map[string]*nodeInfo, []string) {
	filtered := make(map[string]*nodeInfo, len(keep))
	for id := range keep {
		orig := index[id]
		clone := *orig
		kept := make([]string, 0, len(orig.children))
		for _, cid := range orig.children {
			if keep[cid] {
				kept = append(kept, cid)
			}
		}
		clone.children = kept
		filtered[id] = &clone
	}

	var filteredRoots []string
	for _, r := range roots {
		if keep[r] {
			filteredRoots = append(filteredRoots, r)
		}
	}

	// Fallback: if no original roots survived (e.g. "forms"/"main" subtree filters
	// that never mark the RootWebArea), derive roots from kept nodes that have no
	// kept parent — so the rendered tree is always reachable.
	if len(filteredRoots) == 0 && len(filtered) > 0 {
		isKeptChild := make(map[string]bool, len(keep))
		for id := range filtered {
			for _, cid := range filtered[id].children {
				isKeptChild[cid] = true
			}
		}
		for id := range filtered {
			if !isKeptChild[id] {
				filteredRoots = append(filteredRoots, id)
			}
		}
	}

	return filtered, filteredRoots
}
