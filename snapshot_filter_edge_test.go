package browser

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildSimpleIndex creates a minimal index + root list directly (no proto round-trip).
// nodes: slice of [id, role, name] + children list pairs.
func buildDirectIndex(entries []nodeEntry) (map[string]*nodeInfo, []string) {
	index := make(map[string]*nodeInfo, len(entries))
	isChild := make(map[string]bool)
	for _, e := range entries {
		index[e.id] = &nodeInfo{role: e.role, name: e.name, children: e.children}
		for _, cid := range e.children {
			isChild[cid] = true
		}
	}
	var roots []string
	for _, e := range entries {
		if !isChild[e.id] {
			roots = append(roots, e.id)
		}
	}
	return index, roots
}

type nodeEntry struct {
	id, role, name string
	children       []string
}

// ---------------------------------------------------------------------------
// "interactive" filter — edge cases
// ---------------------------------------------------------------------------

// TestFilter_DeepNestedInteractive: button 5 levels deep, all ancestors must survive.
func TestFilter_DeepNestedInteractive(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"l1"}},
		{"l1", "generic", "", []string{"l2"}},
		{"l2", "group", "", []string{"l3"}},
		{"l3", "region", "", []string{"l4"}},
		{"l4", "group", "", []string{"btn"}},
		{"btn", "button", "Deep Submit", nil},
	})

	filtered, _ := filterAXTree(index, roots, "interactive", "")

	for _, id := range []string{"root", "l1", "l2", "l3", "l4", "btn"} {
		if _, ok := filtered[id]; !ok {
			t.Errorf("interactive filter: node %q (deeply nested) should be kept", id)
		}
	}
}

// TestFilter_IframeInsideForm: iframe role "Iframe" must survive interactive filter.
func TestFilter_IframeInsideForm(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1"}},
		{"form1", "form", "Payment", []string{"iframe1"}},
		{"iframe1", "Iframe", "Card input", nil},
	})

	filtered, _ := filterAXTree(index, roots, "interactive", "")

	if _, ok := filtered["iframe1"]; !ok {
		t.Error("interactive filter: Iframe should be kept (critical for payment tokenization)")
	}
	if _, ok := filtered["form1"]; !ok {
		t.Error("interactive filter: form ancestor of Iframe should be kept")
	}
}

// TestFilter_EmptyFormInteractive: form with no children should still survive.
func TestFilter_EmptyFormInteractive(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1", "para1"}},
		{"form1", "form", "Empty Form", nil},
		{"para1", "paragraph", "Some text", nil},
	})

	filtered, _ := filterAXTree(index, roots, "interactive", "")

	if _, ok := filtered["form1"]; !ok {
		t.Error("interactive filter: empty form should be kept (form is in filterableRoles)")
	}
	if _, ok := filtered["para1"]; ok {
		t.Error("interactive filter: plain paragraph should be excluded")
	}
}

// TestFilter_MixedForms: 2 forms, only one with buttons.
// Form with interactive content survives; form with only paragraphs is excluded.
func TestFilter_MixedForms(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1", "form2"}},
		// form1 has a button → both survive
		{"form1", "form", "With Button", []string{"btn1"}},
		{"btn1", "button", "Go", nil},
		// form2 has only paragraph → form2 itself should survive (it's a filterableRole),
		// but para2 should not.
		{"form2", "form", "Text Only", []string{"para2"}},
		{"para2", "paragraph", "Info text", nil},
	})

	filtered, _ := filterAXTree(index, roots, "interactive", "")

	if _, ok := filtered["form1"]; !ok {
		t.Error("interactive filter: form1 (with button) should be kept")
	}
	if _, ok := filtered["btn1"]; !ok {
		t.Error("interactive filter: button inside form1 should be kept")
	}
	// form2 is a filterableRole itself, so it is kept
	if _, ok := filtered["form2"]; !ok {
		t.Error("interactive filter: form2 is a filterableRole and should be kept")
	}
	// para2 is not interactive
	if _, ok := filtered["para2"]; ok {
		t.Error("interactive filter: plain paragraph should be excluded")
	}
}

// TestFilter_RadioGroup: radiogroup with 4 radio children, all should survive.
func TestFilter_RadioGroup(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"rg1"}},
		{"rg1", "radiogroup", "Options", []string{"r1", "r2", "r3", "r4"}},
		{"r1", "radio", "Option A", nil},
		{"r2", "radio", "Option B", nil},
		{"r3", "radio", "Option C", nil},
		{"r4", "radio", "Option D", nil},
	})

	filtered, _ := filterAXTree(index, roots, "interactive", "")

	for _, id := range []string{"rg1", "r1", "r2", "r3", "r4"} {
		if _, ok := filtered[id]; !ok {
			t.Errorf("interactive filter: radio group node %q should be kept", id)
		}
	}
}

// ---------------------------------------------------------------------------
// "forms" filter — edge cases
// ---------------------------------------------------------------------------

// TestFilter_NestedForms: form inside form — both subtrees should survive.
func TestFilter_NestedForms(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form_outer"}},
		{"form_outer", "form", "Outer", []string{"btn_outer", "form_inner"}},
		{"btn_outer", "button", "Submit Outer", nil},
		{"form_inner", "form", "Inner", []string{"txt_inner"}},
		{"txt_inner", "textbox", "Email", nil},
	})

	filtered, _ := filterAXTree(index, roots, "forms", "")

	for _, id := range []string{"form_outer", "btn_outer", "form_inner", "txt_inner"} {
		if _, ok := filtered[id]; !ok {
			t.Errorf("forms filter: nested form node %q should be kept", id)
		}
	}
}

// TestFilter_ButtonOutsideForm: standalone button not in any form should NOT survive forms filter.
func TestFilter_ButtonOutsideForm(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"btn_standalone", "form1"}},
		{"btn_standalone", "button", "Standalone", nil},
		{"form1", "form", "Login", []string{"txt1"}},
		{"txt1", "textbox", "User", nil},
	})

	filtered, _ := filterAXTree(index, roots, "forms", "")

	if _, ok := filtered["btn_standalone"]; ok {
		t.Error("forms filter: standalone button (outside form) should be excluded")
	}
	if _, ok := filtered["form1"]; !ok {
		t.Error("forms filter: form should be kept")
	}
	if _, ok := filtered["txt1"]; !ok {
		t.Error("forms filter: textbox inside form should be kept")
	}
}

// TestFilter_FormWithIframes: form containing iframe children, all should survive.
func TestFilter_FormWithIframes(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1"}},
		{"form1", "form", "Checkout", []string{"txt1", "iframe1", "btn1"}},
		{"txt1", "textbox", "Name", nil},
		{"iframe1", "Iframe", "Card Input", nil},
		{"btn1", "button", "Pay", nil},
	})

	filtered, _ := filterAXTree(index, roots, "forms", "")

	for _, id := range []string{"form1", "txt1", "iframe1", "btn1"} {
		if _, ok := filtered[id]; !ok {
			t.Errorf("forms filter: node %q inside form should be kept", id)
		}
	}
}

// ---------------------------------------------------------------------------
// "main" filter — edge cases
// ---------------------------------------------------------------------------

// TestFilter_NoMainElement: page with no role=main should return empty filtered set.
func TestFilter_NoMainElement(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"nav1", "aside1"}},
		{"nav1", "navigation", "Nav", []string{"link1"}},
		{"link1", "link", "Home", nil},
		{"aside1", "complementary", "Sidebar", nil},
	})

	filtered, filteredRoots := filterAXTree(index, roots, "main", "")

	if len(filtered) != 0 {
		t.Errorf("main filter with no main element: expected empty filtered index, got %d nodes", len(filtered))
	}
	if len(filteredRoots) != 0 {
		t.Errorf("main filter with no main element: expected empty roots, got %d", len(filteredRoots))
	}
}

// TestFilter_MultipleMainElements: two main sections, both subtrees should survive.
func TestFilter_MultipleMainElements(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"main1", "nav1", "main2"}},
		{"main1", "main", "Primary Content", []string{"h1"}},
		{"h1", "heading", "Title", nil},
		{"nav1", "navigation", "Nav", []string{"link1"}},
		{"link1", "link", "Home", nil},
		{"main2", "main", "Secondary Content", []string{"para1"}},
		{"para1", "paragraph", "Text", nil},
	})

	filtered, _ := filterAXTree(index, roots, "main", "")

	for _, id := range []string{"main1", "h1", "main2", "para1"} {
		if _, ok := filtered[id]; !ok {
			t.Errorf("main filter: node %q should be kept (inside a main element)", id)
		}
	}
	for _, id := range []string{"nav1", "link1"} {
		if _, ok := filtered[id]; ok {
			t.Errorf("main filter: node %q should be excluded (outside main element)", id)
		}
	}
}

// ---------------------------------------------------------------------------
// "text" filter — edge cases
// ---------------------------------------------------------------------------

// TestFilter_NavigationStripped: navigation and its children stripped by text filter.
func TestFilter_NavigationStripped(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"nav1", "main1"}},
		{"nav1", "navigation", "Site Nav", []string{"link1", "link2"}},
		{"link1", "link", "About", nil},
		{"link2", "link", "Contact", nil},
		{"main1", "main", "Content", []string{"para1"}},
		{"para1", "paragraph", "Article text", nil},
	})

	filtered, _ := filterAXTree(index, roots, "text", "")

	// navigation and children should be stripped
	for _, id := range []string{"nav1", "link1", "link2"} {
		if _, ok := filtered[id]; ok {
			t.Errorf("text filter: %q (in navigation) should be excluded", id)
		}
	}
	// main content should survive
	for _, id := range []string{"root", "main1", "para1"} {
		if _, ok := filtered[id]; !ok {
			t.Errorf("text filter: %q should be kept", id)
		}
	}
}

// TestFilter_FooterStripped: contentinfo (footer) should be stripped including children.
func TestFilter_FooterStripped(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"main1", "footer1"}},
		{"main1", "main", "Content", []string{"para1"}},
		{"para1", "paragraph", "Main text", nil},
		{"footer1", "contentinfo", "Footer", []string{"copy1", "link3"}},
		{"copy1", "paragraph", "© 2026", nil},
		{"link3", "link", "Privacy Policy", nil},
	})

	filtered, _ := filterAXTree(index, roots, "text", "")

	// footer and children stripped
	for _, id := range []string{"footer1", "copy1", "link3"} {
		if _, ok := filtered[id]; ok {
			t.Errorf("text filter: %q (in contentinfo) should be excluded", id)
		}
	}
	// main content kept
	for _, id := range []string{"root", "main1", "para1"} {
		if _, ok := filtered[id]; !ok {
			t.Errorf("text filter: %q should be kept", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Structural integrity
// ---------------------------------------------------------------------------

// TestFilter_ChildrenListPruning: after filter, children of surviving nodes must not
// reference pruned nodes.
func TestFilter_ChildrenListPruning(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1", "nav1", "aside1"}},
		{"form1", "form", "Login", []string{"btn1"}},
		{"btn1", "button", "Submit", nil},
		{"nav1", "navigation", "Nav", []string{"link1"}},
		{"link1", "link", "Home", nil},
		{"aside1", "complementary", "Sidebar", nil},
	})

	filtered, filteredRoots := filterAXTree(index, roots, "forms", "")
	_ = filteredRoots

	// Check every surviving node's children only reference surviving nodes
	for id, node := range filtered {
		for _, cid := range node.children {
			if _, ok := filtered[cid]; !ok {
				t.Errorf("forms filter: node %q has dangling child reference %q", id, cid)
			}
		}
	}
}

// TestFilter_OriginalNotMutated: filter must clone nodes, original index unchanged.
func TestFilter_OriginalNotMutated(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1", "nav1"}},
		{"form1", "form", "Login", []string{"btn1", "txt1"}},
		{"btn1", "button", "Submit", nil},
		{"txt1", "textbox", "User", nil},
		{"nav1", "navigation", "Nav", []string{"link1"}},
		{"link1", "link", "Home", nil},
	})

	// Capture original state
	origRootChildren := make([]string, len(index["root"].children))
	copy(origRootChildren, index["root"].children)
	origForm1Children := make([]string, len(index["form1"].children))
	copy(origForm1Children, index["form1"].children)

	filterAXTree(index, roots, "forms", "")

	// Original root children list must be unchanged
	if len(index["root"].children) != len(origRootChildren) {
		t.Errorf("filter mutated root.children: was %v, now %v",
			origRootChildren, index["root"].children)
	}
	for i, cid := range origRootChildren {
		if index["root"].children[i] != cid {
			t.Errorf("filter mutated root.children[%d]: was %q, now %q",
				i, cid, index["root"].children[i])
		}
	}
	// form1 children list must be unchanged
	if len(index["form1"].children) != len(origForm1Children) {
		t.Errorf("filter mutated form1.children: was %v, now %v",
			origForm1Children, index["form1"].children)
	}
}

// TestFilter_UnknownFilter: unknown filter value returns original index unchanged.
func TestFilter_UnknownFilter(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"nav1", "btn1"}},
		{"nav1", "navigation", "Nav", []string{"link1"}},
		{"link1", "link", "Home", nil},
		{"btn1", "button", "Submit", nil},
	})

	filtered, filteredRoots := filterAXTree(index, roots, "unknown_filter_xyz", "")

	if len(filtered) != len(index) {
		t.Errorf("unknown filter: want %d nodes, got %d", len(index), len(filtered))
	}
	if len(filteredRoots) != len(roots) {
		t.Errorf("unknown filter: want %d roots, got %d", len(roots), len(filteredRoots))
	}
}

// ---------------------------------------------------------------------------
// Integration with rendering
// ---------------------------------------------------------------------------

// TestFilter_RenderingIntegration: filter then renderYAML — output contains only expected roles.
func TestFilter_RenderingIntegration(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1", "nav1"}},
		{"form1", "form", "Login", []string{"txt1", "btn1"}},
		{"txt1", "textbox", "Username", nil},
		{"btn1", "button", "Sign In", nil},
		{"nav1", "navigation", "Nav", []string{"link1"}},
		{"link1", "link", "About", nil},
	})

	filtered, filteredRoots := filterAXTree(index, roots, "interactive", "")
	output := renderYAML(filtered, filteredRoots, 0)

	// Should contain interactive elements
	if !strings.Contains(output, "textbox") {
		t.Errorf("rendered output should contain textbox, got:\n%s", output)
	}
	if !strings.Contains(output, "button") {
		t.Errorf("rendered output should contain button, got:\n%s", output)
	}
	if !strings.Contains(output, "link") {
		t.Errorf("rendered output should contain link, got:\n%s", output)
	}
	// Should NOT contain navigation role as content (navigation IS kept as ancestor of link)
	// But should not contain plain paragraph
	if strings.Contains(output, "paragraph") {
		t.Errorf("rendered output should not contain paragraph after interactive filter, got:\n%s", output)
	}
}

// TestFilter_FormsRenderingHasContent: forms filter + rendering should produce non-empty output.
// This is a regression test for the filteredRoots bug (if roots are excluded, output is empty).
func TestFilter_FormsRenderingHasContent(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1", "nav1"}},
		{"form1", "form", "Login", []string{"txt1", "btn1"}},
		{"txt1", "textbox", "User", nil},
		{"btn1", "button", "Submit", nil},
		{"nav1", "navigation", "Nav", []string{"link1"}},
		{"link1", "link", "Home", nil},
	})

	filtered, filteredRoots := filterAXTree(index, roots, "forms", "")
	output := renderYAML(filtered, filteredRoots, 0)

	if strings.TrimSpace(output) == "" {
		t.Error("forms filter: renderYAML output should not be empty (filteredRoots may be missing)")
	}
	if !strings.Contains(output, "form") {
		t.Errorf("forms filter: rendered output should contain form element, got:\n%s", output)
	}
	if !strings.Contains(output, "textbox") {
		t.Errorf("forms filter: rendered output should contain textbox, got:\n%s", output)
	}
}
