package browser

import (
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

// buildTestAXNodes creates a small AX tree for testing extraction levels:
//
//	RootWebArea "Page"
//	├── navigation
//	│   ├── link "Home"
//	│   └── link "About"
//	├── main
//	│   ├── heading "Welcome" (level=1)
//	│   ├── paragraph "Lorem ipsum"
//	│   └── form "Login"
//	│       ├── textbox "Email"
//	│       └── button "Submit"
//	└── contentinfo
//	    └── text "Copyright"
func buildTestAXNodes() []*proto.AccessibilityAXNode {
	mkLevel := func(level int) []*proto.AccessibilityAXProperty {
		return []*proto.AccessibilityAXProperty{
			{
				Name:  "level",
				Value: makeAXValue(float64(level)),
			},
		}
	}

	return []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"nav", "main", "footer"}),
		makeAXNode("nav", "navigation", "", nil, []string{"link1", "link2"}),
		makeAXNode("link1", "link", "Home", nil, nil),
		makeAXNode("link2", "link", "About", nil, nil),
		makeAXNode("main", "main", "", nil, []string{"heading1", "para1", "form1"}),
		makeAXNode("heading1", "heading", "Welcome", mkLevel(1), nil),
		makeAXNode("para1", "paragraph", "Lorem ipsum", nil, nil),
		makeAXNode("form1", "form", "Login", nil, []string{"email", "submit"}),
		makeAXNode("email", "textbox", "Email", nil, nil),
		makeAXNode("submit", "button", "Submit", nil, nil),
		makeAXNode("footer", "contentinfo", "", nil, []string{"copyright"}),
		makeAXNode("copyright", "StaticText", "Copyright", nil, nil),
	}
}

func TestExtractLevel_Skeleton(t *testing.T) {
	nodes := buildTestAXNodes()
	index, roots := buildAXIndex(nodes)
	index, roots = applyLevelFilter(index, roots, LevelSkeleton)

	// Skeleton should keep only interactive roles + ancestors.
	// Interactive: link1, link2, email, submit
	// Ancestors: root, nav, main, form1
	expected := map[string]bool{
		"root": true, "nav": true, "link1": true, "link2": true,
		"main": true, "form1": true, "email": true, "submit": true,
	}
	for id := range index {
		if !expected[id] {
			t.Errorf("skeleton: unexpected node %q (role=%s) in filtered index", id, index[id].role)
		}
	}
	for id := range expected {
		if _, ok := index[id]; !ok {
			t.Errorf("skeleton: expected node %q missing from filtered index", id)
		}
	}
	_ = roots
}

func TestExtractLevel_Content(t *testing.T) {
	nodes := buildTestAXNodes()
	index, roots := buildAXIndex(nodes)
	index, roots = applyLevelFilter(index, roots, LevelContent)

	// Content = skeleton + content roles (navigation, main, form, heading, contentinfo).
	// Should now also include heading1 and footer.
	// paragraph is NOT in skeleton or content roles, so it should be excluded.
	if _, ok := index["heading1"]; !ok {
		t.Error("content: heading1 should be present")
	}
	if _, ok := index["footer"]; !ok {
		t.Error("content: footer (contentinfo) should be present")
	}
	if _, ok := index["para1"]; ok {
		t.Error("content: paragraph should NOT be present (not in skeleton or content roles)")
	}
	_ = roots
}

func TestExtractLevel_Full(t *testing.T) {
	nodes := buildTestAXNodes()
	index, _ := buildAXIndex(nodes)
	fullIndex, _ := buildAXIndex(nodes)
	index, _ = applyLevelFilter(index, nil, LevelFull)

	// Full should keep everything (no filtering).
	if len(index) != len(fullIndex) {
		t.Errorf("full: expected %d nodes, got %d", len(fullIndex), len(index))
	}
}

func TestParseExtractLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected ExtractLevel
		ok       bool
	}{
		{"skeleton", LevelSkeleton, true},
		{"content", LevelContent, true},
		{"full", LevelFull, true},
		{"SKELETON", LevelSkeleton, true}, // case-insensitive
		{"Content", LevelContent, true},
		{"interactive", "", false}, // legacy filter, not a level
		{"", "", false},
	}
	for _, tc := range tests {
		got, ok := parseExtractLevel(tc.input)
		if ok != tc.ok {
			t.Errorf("parseExtractLevel(%q): ok=%v, want %v", tc.input, ok, tc.ok)
		}
		if ok && got != tc.expected {
			t.Errorf("parseExtractLevel(%q): got %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestShouldIncludeNode(t *testing.T) {
	tests := []struct {
		role  string
		name  string
		level ExtractLevel
		want  bool
	}{
		{"button", "Click", LevelSkeleton, true},
		{"link", "Home", LevelSkeleton, true},
		{"navigation", "", LevelSkeleton, false},
		{"navigation", "", LevelContent, true},
		{"heading", "Title", LevelContent, true},
		{"paragraph", "Text", LevelContent, false},
		{"paragraph", "Text", LevelFull, true}, // has name
		{"generic", "", LevelFull, false},      // generic always excluded
		{"", "name", LevelFull, false},         // empty role excluded
	}
	for _, tc := range tests {
		got := shouldIncludeNode(tc.role, tc.name, tc.level)
		if got != tc.want {
			t.Errorf("shouldIncludeNode(%q, %q, %v): got %v, want %v", tc.role, tc.name, tc.level, got, tc.want)
		}
	}
}

// TestRenderAXTreeWithLevel_SkeletonTokenReduction verifies that skeleton level
// produces fewer lines than full level — the core value proposition.
func TestRenderAXTreeWithLevel_SkeletonTokenReduction(t *testing.T) {
	nodes := buildTestAXNodes()

	skeletonOut := renderAXTreeWithLevel(nodes, 0, "text", nil, "", LevelSkeleton, nil)
	fullOut := renderAXTreeWithLevel(nodes, 0, "text", nil, "", LevelFull, nil)

	skeletonLines := strings.Count(skeletonOut, "\n")
	fullLines := strings.Count(fullOut, "\n")

	if skeletonLines >= fullLines {
		t.Errorf("skeleton (%d lines) should be smaller than full (%d lines)", skeletonLines, fullLines)
	}
	t.Logf("skeleton=%d lines, full=%d lines (%.0f%% reduction)",
		skeletonLines, fullLines, float64(fullLines-skeletonLines)/float64(fullLines)*100)
}
