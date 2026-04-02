package browser

import (
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

func TestRenderYAML_InlineText(t *testing.T) {
	// Paragraph with StaticText child should produce inline text.
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"p1"}),
		makeAXNode("p1", "paragraph", "", nil, []string{"st1"}),
		makeAXNode("st1", "StaticText", "Hello world", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 0)

	if !strings.Contains(got, "- paragraph: Hello world") {
		t.Errorf("expected inline text on paragraph, got:\n%s", got)
	}
	if strings.Contains(got, "StaticText") {
		t.Errorf("StaticText should not appear in output, got:\n%s", got)
	}
}

func TestRenderYAML_InlineTextMultiple(t *testing.T) {
	// Multiple StaticText children should be joined.
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"p1"}),
		makeAXNode("p1", "paragraph", "", nil, []string{"st1", "st2"}),
		makeAXNode("st1", "StaticText", "Hello", nil, nil),
		makeAXNode("st2", "StaticText", "world", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 0)

	if !strings.Contains(got, "- paragraph: Hello world") {
		t.Errorf("expected joined inline text, got:\n%s", got)
	}
}

func TestRenderYAML_TextWithChildren(t *testing.T) {
	// Node with both text and real children renders text as child.
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"btn"}),
		makeAXNode("btn", "button", "Submit", nil, []string{"st1", "img1"}),
		makeAXNode("st1", "StaticText", "Click here", nil, nil),
		makeAXNode("img1", "img", "icon", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 0)

	if !strings.Contains(got, `- button "Submit" [ref=e1] [cursor=pointer]:`) {
		t.Errorf("expected button with colon, got:\n%s", got)
	}
	if !strings.Contains(got, "- text: Click here") {
		t.Errorf("expected text child line, got:\n%s", got)
	}
	if !strings.Contains(got, `- img "icon"`) {
		t.Errorf("expected img child, got:\n%s", got)
	}
}

func TestRenderYAML_GenericWithName(t *testing.T) {
	// Generic with a name should render (not be collapsed).
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"gen"}),
		makeAXNode("gen", "generic", "Label", nil, []string{"btn"}),
		makeAXNode("btn", "button", "OK", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 0)

	if !strings.Contains(got, `generic "Label"`) {
		t.Errorf("named generic should appear, got:\n%s", got)
	}
	if !strings.Contains(got, `button "OK"`) {
		t.Errorf("button should appear, got:\n%s", got)
	}
}

func TestRenderYAML_GenericWithText(t *testing.T) {
	// Generic with text from StaticText child should render.
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"gen"}),
		makeAXNode("gen", "generic", "", nil, []string{"st1"}),
		makeAXNode("st1", "StaticText", "Some content", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 0)

	if !strings.Contains(got, "- generic: Some content") {
		t.Errorf("generic with text should render inline, got:\n%s", got)
	}
}

func TestRenderYAML_TreeNesting(t *testing.T) {
	// Verify proper indentation with nested structure.
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"nav"}),
		makeAXNode("nav", "navigation", "", nil, []string{"list"}),
		makeAXNode("list", "list", "", nil, []string{"item1"}),
		makeAXNode("item1", "listitem", "", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Home", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 0)

	// Check that nesting is preserved: each level indented by 2 spaces.
	lines := strings.Split(strings.TrimSpace(got), "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, `link "Home"`) {
			found = true
			// Should be indented at level 4 (root > nav > list > listitem > link).
			indent := len(line) - len(strings.TrimLeft(line, " "))
			if indent != 8 { // 4 levels * 2 spaces
				t.Errorf("link should be at indent 8, got %d in:\n%s", indent, got)
			}
		}
	}
	if !found {
		t.Errorf("link not found in output:\n%s", got)
	}
}

func TestRenderYAML_EmptyGenericLeaf(t *testing.T) {
	// Empty generic leaf (no children, no name, no text) should be skipped.
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"gen", "btn"}),
		makeAXNode("gen", "generic", "", nil, nil),
		makeAXNode("btn", "button", "OK", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 0)

	if strings.Contains(got, "generic") {
		t.Errorf("empty generic leaf should be skipped, got:\n%s", got)
	}
	if !strings.Contains(got, `button "OK"`) {
		t.Errorf("button should appear, got:\n%s", got)
	}
}

func TestRenderYAML_LinkURL(t *testing.T) {
	// Build index directly to test /url: rendering.
	index := map[string]*nodeInfo{
		"root":  {role: "RootWebArea", name: "Page", children: []string{"link1", "link2"}},
		"link1": {role: "link", name: "Home", url: "/home"},
		"link2": {role: "link", name: "Settings", url: "/settings"},
	}
	roots := []string{"root"}

	got := renderYAML(index, roots, 0)

	if !strings.Contains(got, "- /url: /home") {
		t.Errorf("expected /url: /home, got:\n%s", got)
	}
	if !strings.Contains(got, "- /url: /settings") {
		t.Errorf("expected /url: /settings, got:\n%s", got)
	}
	// Links with URL should have colon (children block).
	if !strings.Contains(got, `link "Home" [ref=e1] [cursor=pointer]:`) {
		t.Errorf("link with url should have colon, got:\n%s", got)
	}
}

func TestRenderYAML_LinkWithoutURL(t *testing.T) {
	// Link without url should not produce /url: line.
	index := map[string]*nodeInfo{
		"root":  {role: "RootWebArea", name: "Page", children: []string{"link1"}},
		"link1": {role: "link", name: "Home"},
	}
	roots := []string{"root"}

	got := renderYAML(index, roots, 0)

	if strings.Contains(got, "/url:") {
		t.Errorf("link without url should not have /url:, got:\n%s", got)
	}
}

func TestRenderYAML_LinkURLWithText(t *testing.T) {
	// Link with both text and URL should render both as children.
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Home", nil, []string{"st1"}),
		makeAXNode("st1", "StaticText", "Go home", nil, nil),
	}

	index, roots := buildAXIndex(nodes)
	// Simulate URL being applied.
	index["link1"].url = "/home"

	got := renderYAML(index, roots, 0)

	if !strings.Contains(got, "- text: Go home") {
		t.Errorf("expected text child, got:\n%s", got)
	}
	if !strings.Contains(got, "- /url: /home") {
		t.Errorf("expected /url child, got:\n%s", got)
	}
}
