package browser

import "strings"

// ExtractLevel controls how much of the accessibility tree is returned by doSnapshot.
// It maps to the ghostchrome/bonk pattern: skeleton (interactive only),
// content (skeleton + landmarks + headings), full (everything with a name).
//
// #63: Token-lean a11y tree — skeleton/content/full extraction levels.
// Reduces token usage 3-5× for AI-agent consumers (go-wowa MCP).
type ExtractLevel string

const (
	// LevelSkeleton returns only interactive elements (buttons, links, inputs,
	// tabs, menu items). Minimal token output — ideal for agent action loops
	// where only actionable elements matter.
	LevelSkeleton ExtractLevel = "skeleton"

	// LevelContent returns skeleton + structural/landmark roles (navigation,
	// main, form, search, banner, headings). Default for AI-agent consumers —
	// gives the agent enough context to understand page structure without
	// dumping every node.
	LevelContent ExtractLevel = "content"

	// LevelFull returns everything with a non-empty name or known role.
	// Equivalent to no filtering — use for debugging or full-page analysis.
	LevelFull ExtractLevel = "full"
)

// skeletonRoles are the roles kept in LevelSkeleton mode.
// Only actionable elements — the minimal set an agent needs to interact.
var skeletonRoles = map[string]bool{
	"link": true, "button": true, "textbox": true, "combobox": true,
	"checkbox": true, "radio": true, "slider": true, "switch": true,
	"tab": true, "menuitem": true, "option": true, "searchbox": true,
	"spinbutton": true, "treeitem": true, "menuitemcheckbox": true,
	"menuitemradio": true, "listbox": true,
}

// contentRoles are the additional structural/landmark roles kept in LevelContent
// mode (on top of skeletonRoles).
var contentRoles = map[string]bool{
	"navigation": true, "form": true, "search": true,
	"banner": true, "main": true, "complementary": true,
	"contentinfo": true, "heading": true, "dialog": true,
	"alertdialog": true, "alert": true, "status": true,
	"region": true, "article": true, "feed": true,
}

// shouldIncludeNode decides whether a node with the given role and name should
// be included at the given extraction level. Non-interactive nodes are still
// traversed (their children might be relevant) — this only controls whether the
// node itself gets a line in the output.
func shouldIncludeNode(role, name string, level ExtractLevel) bool {
	// Skip generic/none roles unless they have meaningful content.
	if role == "" || role == "none" || role == "generic" {
		return false
	}

	switch level {
	case LevelSkeleton:
		return skeletonRoles[role]
	case LevelContent:
		return skeletonRoles[role] || contentRoles[role]
	case LevelFull:
		// Include everything with a non-empty name or a known role.
		return name != "" || skeletonRoles[role] || contentRoles[role]
	}
	return false
}

// levelToFilter converts an ExtractLevel to the existing filter system's filter
// string, for backward compatibility with filterAXTree. This lets Level and
// Filter coexist — Level is the new preferred API, Filter is the legacy one.
func levelToFilter(level ExtractLevel) string {
	switch level {
	case LevelSkeleton:
		return "interactive"
	case LevelContent:
		// No exact legacy equivalent — "interactive" is too narrow, "" is too wide.
		// We handle content-level filtering in applyLevelFilter instead.
		return "__content__"
	case LevelFull:
		return ""
	}
	return ""
}

// applyLevelFilter filters the AX tree according to an ExtractLevel.
// Returns a filtered index and roots. For skeleton, this delegates to the
// existing interactive filter. For content, it keeps skeleton + content roles
// plus their ancestors. For full, it's a no-op.
func applyLevelFilter(
	index map[string]*nodeInfo,
	roots []string,
	level ExtractLevel,
) (map[string]*nodeInfo, []string) {
	switch level {
	case LevelFull, "":
		return index, roots
	case LevelSkeleton:
		return filterAXTree(index, roots, "interactive", "")
	case LevelContent:
		return applyContentLevelFilter(index, roots)
	}
	return index, roots
}

// applyContentLevelFilter keeps skeleton + content roles and their ancestors.
func applyContentLevelFilter(index map[string]*nodeInfo, roots []string) (map[string]*nodeInfo, []string) {
	keep := make(map[string]bool, len(index))
	parent := buildParentMap(index, roots)

	for id, n := range index {
		if skeletonRoles[n.role] || contentRoles[n.role] {
			markWithAncestors(id, index, parent, keep)
		}
	}

	return buildFilteredIndex(index, roots, keep)
}

// parseExtractLevel parses a level string, returning the level and a bool
// indicating whether the string was a valid level (vs. a legacy filter).
func parseExtractLevel(s string) (ExtractLevel, bool) {
	switch strings.ToLower(s) {
	case "skeleton":
		return LevelSkeleton, true
	case "content":
		return LevelContent, true
	case "full":
		return LevelFull, true
	}
	return "", false
}
