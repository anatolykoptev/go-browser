package browser

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

// buildTestIndex is a helper that runs buildAXIndex on a slice of nodes.
func buildTestIndex(nodes []*proto.AccessibilityAXNode) (map[string]*nodeInfo, []string) {
	return buildAXIndex(nodes)
}

// TestFilterAXTree_Empty verifies that empty filter is a no-op.
func TestFilterAXTree_Empty(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"btn1", "nav1"}),
		makeAXNode("btn1", "button", "Submit", nil, nil),
		makeAXNode("nav1", "navigation", "Nav", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Home", nil, nil),
	}
	index, roots := buildTestIndex(nodes)
	filtered, filteredRoots := filterAXTree(index, roots, "", "")

	// Should return same maps (identity).
	if len(filtered) != len(index) {
		t.Errorf("empty filter: want %d nodes, got %d", len(index), len(filtered))
	}
	if len(filteredRoots) != len(roots) {
		t.Errorf("empty filter: want %d roots, got %d", len(roots), len(filteredRoots))
	}
}

// TestFilterAXTree_Interactive verifies that "interactive" mode keeps buttons/links and their ancestors.
func TestFilterAXTree_Interactive(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"main1", "nav1"}),
		makeAXNode("main1", "main", "Main", nil, []string{"btn1", "para1"}),
		makeAXNode("btn1", "button", "Submit", nil, nil),
		makeAXNode("para1", "paragraph", "", nil, []string{"st1"}),
		makeAXNode("st1", "StaticText", "Some text", nil, nil),
		makeAXNode("nav1", "navigation", "Nav", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Home", nil, nil),
	}
	index, roots := buildTestIndex(nodes)
	filtered, _ := filterAXTree(index, roots, "interactive", "")

	// Button and link should be kept.
	if _, ok := filtered["btn1"]; !ok {
		t.Error("interactive filter: button should be kept")
	}
	if _, ok := filtered["link1"]; !ok {
		t.Error("interactive filter: link should be kept")
	}
	// Ancestors (root, main1, nav1) should be kept.
	if _, ok := filtered["root"]; !ok {
		t.Error("interactive filter: root should be kept as ancestor")
	}
	if _, ok := filtered["main1"]; !ok {
		t.Error("interactive filter: main should be kept as ancestor of button")
	}
	if _, ok := filtered["nav1"]; !ok {
		t.Error("interactive filter: navigation should be kept as ancestor of link")
	}
	// Plain paragraph (no interactive content) should NOT be kept.
	// (para1 was cleaned in buildAXIndex, st1 was a noise node).
	if _, ok := filtered["para1"]; ok {
		t.Error("interactive filter: empty paragraph should not be kept")
	}
}

// TestFilterAXTree_Forms verifies that "forms" mode keeps form subtrees only.
func TestFilterAXTree_Forms(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"form1", "nav1"}),
		makeAXNode("form1", "form", "Login", nil, []string{"txt1", "btn1"}),
		makeAXNode("txt1", "textbox", "Username", nil, nil),
		makeAXNode("btn1", "button", "Login", nil, nil),
		makeAXNode("nav1", "navigation", "Nav", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Home", nil, nil),
	}
	index, roots := buildTestIndex(nodes)
	filtered, _ := filterAXTree(index, roots, "forms", "")

	// form1 and its children should be kept.
	if _, ok := filtered["form1"]; !ok {
		t.Error("forms filter: form node should be kept")
	}
	if _, ok := filtered["txt1"]; !ok {
		t.Error("forms filter: textbox inside form should be kept")
	}
	if _, ok := filtered["btn1"]; !ok {
		t.Error("forms filter: button inside form should be kept")
	}
	// nav1 and link1 should be excluded.
	if _, ok := filtered["nav1"]; ok {
		t.Error("forms filter: navigation should be excluded")
	}
	if _, ok := filtered["link1"]; ok {
		t.Error("forms filter: link outside form should be excluded")
	}
}

// TestFilterAXTree_Main verifies that "main" mode keeps main content subtrees.
func TestFilterAXTree_Main(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"main1", "aside1"}),
		makeAXNode("main1", "main", "Content", nil, []string{"h1"}),
		makeAXNode("h1", "heading", "Title", nil, nil),
		makeAXNode("aside1", "complementary", "Sidebar", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Ad", nil, nil),
	}
	index, roots := buildTestIndex(nodes)
	filtered, _ := filterAXTree(index, roots, "main", "")

	if _, ok := filtered["main1"]; !ok {
		t.Error("main filter: main node should be kept")
	}
	if _, ok := filtered["h1"]; !ok {
		t.Error("main filter: heading inside main should be kept")
	}
	if _, ok := filtered["aside1"]; ok {
		t.Error("main filter: sidebar should be excluded")
	}
	if _, ok := filtered["link1"]; ok {
		t.Error("main filter: link in sidebar should be excluded")
	}
}

// TestFilterAXTree_Text verifies that "text" mode excludes navigation/banner/contentinfo.
func TestFilterAXTree_Text(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"banner1", "main1", "footer1"}),
		makeAXNode("banner1", "banner", "Header", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Logo", nil, nil),
		makeAXNode("main1", "main", "Content", nil, []string{"para1"}),
		makeAXNode("para1", "paragraph", "Body text", nil, nil),
		makeAXNode("footer1", "contentinfo", "Footer", nil, []string{"link2"}),
		makeAXNode("link2", "link", "Privacy", nil, nil),
	}
	index, roots := buildTestIndex(nodes)
	filtered, _ := filterAXTree(index, roots, "text", "")

	// root and main should be kept.
	if _, ok := filtered["root"]; !ok {
		t.Error("text filter: root should be kept")
	}
	if _, ok := filtered["main1"]; !ok {
		t.Error("text filter: main should be kept")
	}
	if _, ok := filtered["para1"]; !ok {
		t.Error("text filter: paragraph should be kept")
	}
	// banner and contentinfo subtrees should be excluded.
	if _, ok := filtered["banner1"]; ok {
		t.Error("text filter: banner should be excluded")
	}
	if _, ok := filtered["link1"]; ok {
		t.Error("text filter: link inside banner should be excluded")
	}
	if _, ok := filtered["footer1"]; ok {
		t.Error("text filter: contentinfo should be excluded")
	}
	if _, ok := filtered["link2"]; ok {
		t.Error("text filter: link inside contentinfo should be excluded")
	}
}

// TestFilterAXTree_Selector verifies that selector narrows results by name match.
func TestFilterAXTree_Selector(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"btn1", "btn2"}),
		makeAXNode("btn1", "button", "Submit Form", nil, nil),
		makeAXNode("btn2", "button", "Cancel", nil, nil),
	}
	index, roots := buildTestIndex(nodes)
	filtered, _ := filterAXTree(index, roots, "", "submit")

	if _, ok := filtered["btn1"]; !ok {
		t.Error("selector filter: 'Submit Form' button should match 'submit'")
	}
	if _, ok := filtered["btn2"]; ok {
		t.Error("selector filter: 'Cancel' button should not match 'submit'")
	}
}

// TestFilterAXTree_ChildrenPruned verifies that filtered-out children are removed from parent.
func TestFilterAXTree_ChildrenPruned(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"form1", "nav1"}),
		makeAXNode("form1", "form", "Login", nil, []string{"btn1"}),
		makeAXNode("btn1", "button", "Submit", nil, nil),
		makeAXNode("nav1", "navigation", "Nav", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Home", nil, nil),
	}
	index, roots := buildTestIndex(nodes)
	filtered, _ := filterAXTree(index, roots, "forms", "")

	// root should not have nav1 in its children after filtering.
	rootNode, ok := filtered["root"]
	if ok {
		for _, cid := range rootNode.children {
			if cid == "nav1" {
				t.Error("forms filter: nav1 should be pruned from root's children")
			}
		}
	}
}

// TestBuildFilteredIndex_IsolatesOriginal verifies that filtering does not mutate the original index.
func TestBuildFilteredIndex_IsolatesOriginal(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"form1", "nav1"}),
		makeAXNode("form1", "form", "Login", nil, []string{"btn1"}),
		makeAXNode("btn1", "button", "Submit", nil, nil),
		makeAXNode("nav1", "navigation", "Nav", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Home", nil, nil),
	}
	index, roots := buildTestIndex(nodes)
	origRootChildren := len(index["root"].children)

	filterAXTree(index, roots, "forms", "")

	// Original index should be unchanged.
	if len(index["root"].children) != origRootChildren {
		t.Errorf("filterAXTree must not mutate original index: root had %d children, now has %d",
			origRootChildren, len(index["root"].children))
	}
}
