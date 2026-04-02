package browser

import (
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

func makeAXValue(val interface{}) *proto.AccessibilityAXValue {
	return &proto.AccessibilityAXValue{
		Value: gson.New(val),
	}
}

func makeAXNode(id, role, name string, props []*proto.AccessibilityAXProperty, children []string) *proto.AccessibilityAXNode {
	node := &proto.AccessibilityAXNode{
		NodeID: proto.AccessibilityAXNodeID(id),
		Role:   makeAXValue(role),
		Name:   makeAXValue(name),
	}
	node.Properties = props
	for _, cid := range children {
		node.ChildIDs = append(node.ChildIDs, proto.AccessibilityAXNodeID(cid))
	}
	return node
}

func TestExtractProps(t *testing.T) {
	props := []*proto.AccessibilityAXProperty{
		{Name: "focused", Value: makeAXValue(true)},
		{Name: "disabled", Value: makeAXValue(false)},
		{Name: "checked", Value: makeAXValue(true)},
		{Name: "expanded", Value: makeAXValue(false)},
		{Name: "selected", Value: makeAXValue(true)},
		{Name: "required", Value: makeAXValue(true)},
		{Name: "readonly", Value: makeAXValue(false)},
		{Name: "level", Value: makeAXValue(float64(3))},
		{Name: "haspopup", Value: makeAXValue("menu")},
		{Name: "invalid", Value: makeAXValue("grammar")},
		{Name: "autocomplete", Value: makeAXValue("list")},
	}

	node := makeAXNode("1", "button", "Submit", props, []string{"2", "3"})
	node.Value = makeAXValue("submit-value")
	node.Description = makeAXValue("Submit the form")

	info := extractNodeInfo(node)

	if info.role != "button" {
		t.Errorf("role = %q, want %q", info.role, "button")
	}
	if info.name != "Submit" {
		t.Errorf("name = %q, want %q", info.name, "Submit")
	}
	if info.value != "submit-value" {
		t.Errorf("value = %q, want %q", info.value, "submit-value")
	}
	if info.description != "Submit the form" {
		t.Errorf("description = %q, want %q", info.description, "Submit the form")
	}
	if !info.focused {
		t.Error("focused should be true")
	}
	if info.disabled {
		t.Error("disabled should be false")
	}
	if !info.checked {
		t.Error("checked should be true")
	}
	if info.expanded {
		t.Error("expanded should be false")
	}
	if !info.selected {
		t.Error("selected should be true")
	}
	if !info.required {
		t.Error("required should be true")
	}
	if info.readonly {
		t.Error("readonly should be false")
	}
	if info.level != 3 {
		t.Errorf("level = %d, want 3", info.level)
	}
	if info.hasPopup != "menu" {
		t.Errorf("hasPopup = %q, want %q", info.hasPopup, "menu")
	}
	if info.invalid != "grammar" {
		t.Errorf("invalid = %q, want %q", info.invalid, "grammar")
	}
	if info.autoComplete != "list" {
		t.Errorf("autoComplete = %q, want %q", info.autoComplete, "list")
	}
	if len(info.children) != 2 || info.children[0] != "2" || info.children[1] != "3" {
		t.Errorf("children = %v, want [2 3]", info.children)
	}
}

func TestIsNoiseNode(t *testing.T) {
	noiseRoles := []string{"StaticText", "InlineTextBox", "none", "LineBreak"}
	for _, role := range noiseRoles {
		if !isNoiseRole(role) {
			t.Errorf("isNoiseRole(%q) = false, want true", role)
		}
	}

	realRoles := []string{"button", "link", "heading", "textbox", "combobox", "listitem", ""}
	for _, role := range realRoles {
		if isNoiseRole(role) {
			t.Errorf("isNoiseRole(%q) = true, want false", role)
		}
	}
}

func TestRenderYAML(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Test Page", nil, []string{"nav", "form", "h1"}),
		makeAXNode("nav", "navigation", "", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Home", nil, nil),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("form", "form", "Login", nil, []string{"input1", "btn1"})
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("input1", "textbox", "Username", []*proto.AccessibilityAXProperty{
				{Name: "focused", Value: makeAXValue(true)},
				{Name: "required", Value: makeAXValue(true)},
			}, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("btn1", "button", "Sign In", []*proto.AccessibilityAXProperty{
				{Name: "disabled", Value: makeAXValue(true)},
			}, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("h1", "heading", "Welcome", []*proto.AccessibilityAXProperty{
				{Name: "level", Value: makeAXValue(float64(1))},
			}, nil)
			return n
		}(),
	}

	got := renderAXTreeYAML(nodes, 0)

	checks := []struct {
		desc, pattern string
	}{
		{"root with children", `- RootWebArea "Test Page":`},
		{"navigation", `- navigation:`},
		{"link with ref", `- link "Home" [ref=e1] [cursor=pointer]`},
		{"form", `- form "Login":`},
		{"textbox with ref and attrs", `- textbox "Username" [ref=e2] [cursor=pointer] [focused] [required]`},
		{"button disabled", `- button "Sign In" [ref=e3] [cursor=pointer] [disabled]`},
		{"heading level", `- heading "Welcome" [level=1]`},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.pattern) {
			t.Errorf("%s: expected %q in output:\n%s", c.desc, c.pattern, got)
		}
	}
}

func TestRenderYAMLGenericCollapse(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"gen"}),
		makeAXNode("gen", "generic", "", nil, []string{"btn"}),
		makeAXNode("btn", "button", "OK", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 0)

	// Generic node should be collapsed — button at indent level 1, not 2.
	if strings.Contains(got, "generic") {
		t.Errorf("generic node should be collapsed, got:\n%s", got)
	}
	if !strings.Contains(got, `  - button "OK" [ref=e1] [cursor=pointer]`) {
		t.Errorf("button should be at indent 1, got:\n%s", got)
	}
}

func TestRenderYAMLDescription(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("root", "button", "Submit", nil, nil)
			n.Description = makeAXValue("Submit the form")
			return n
		}(),
	}

	got := renderAXTreeYAML(nodes, 0)

	if !strings.Contains(got, "- /description: Submit the form") {
		t.Errorf("expected description line, got:\n%s", got)
	}
}

func TestRenderYAMLValue(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("root", "combobox", "Country", nil, nil)
			n.Value = makeAXValue("Russia")
			return n
		}(),
	}

	got := renderAXTreeYAML(nodes, 0)

	if !strings.Contains(got, `[value="Russia"]`) {
		t.Errorf("expected value attribute, got:\n%s", got)
	}
}

func TestRenderYAMLMaxDepth(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"nav"}),
		makeAXNode("nav", "navigation", "", nil, []string{"link1"}),
		makeAXNode("link1", "link", "Deep", nil, nil),
	}

	got := renderAXTreeYAML(nodes, 2)

	if !strings.Contains(got, "navigation") {
		t.Errorf("depth 1 node should appear, got:\n%s", got)
	}
	if strings.Contains(got, "Deep") {
		t.Errorf("depth 2 node should be cut by maxDepth=2, got:\n%s", got)
	}
}

func TestRenderYAML_ComplexPage(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Complex Page", nil, []string{"nav", "form", "table"}),
		// Navigation with link
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("nav", "navigation", "", nil, []string{"link1"})
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("link1", "link", "Products", nil, nil)
			return n
		}(),
		// Form with various controls
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("form", "form", "Feedback", nil, []string{"input1", "input2", "btn1"})
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("input1", "textbox", "Email", []*proto.AccessibilityAXProperty{
				{Name: "focused", Value: makeAXValue(true)},
				{Name: "required", Value: makeAXValue(true)},
			}, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("input2", "textbox", "Message", []*proto.AccessibilityAXProperty{
				{Name: "readonly", Value: makeAXValue(true)},
			}, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("btn1", "button", "Send", []*proto.AccessibilityAXProperty{
				{Name: "disabled", Value: makeAXValue(true)},
			}, nil)
			return n
		}(),
		// Table with rows and cells
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("table", "table", "Data", nil, []string{"row1", "row2"})
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("row1", "row", "", nil, []string{"cell1a", "cell1b"})
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("cell1a", "cell", "Header 1", nil, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("cell1b", "cell", "Header 2", nil, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("row2", "row", "", nil, []string{"cell2a", "cell2b"})
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("cell2a", "cell", "Data 1", nil, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("cell2b", "cell", "Data 2", nil, nil)
			return n
		}(),
	}

	got := renderAXTreeYAML(nodes, 0)

	checks := []struct {
		desc, pattern string
	}{
		{"root element", `- RootWebArea "Complex Page":`},
		{"navigation role", `- navigation:`},
		{"link with ref", `- link "Products" [ref=e1] [cursor=pointer]`},
		{"form role", `- form "Feedback":`},
		{"focused textbox", `- textbox "Email" [ref=e2] [cursor=pointer] [focused] [required]`},
		{"readonly textbox", `- textbox "Message" [ref=e3] [cursor=pointer] [readonly]`},
		{"disabled button", `- button "Send" [ref=e4] [cursor=pointer] [disabled]`},
		{"table", `- table "Data":`},
		{"row", `- row:`},
		{"cell", `- cell`},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.pattern) {
			t.Errorf("%s: expected %q in output:\n%s", c.desc, c.pattern, got)
		}
	}
}

func TestExtractProps_HiddenLiveModal(t *testing.T) {
	props := []*proto.AccessibilityAXProperty{
		{Name: "hidden", Value: makeAXValue(true)},
		{Name: "live", Value: makeAXValue("polite")},
		{Name: "modal", Value: makeAXValue(true)},
	}
	node := makeAXNode("1", "dialog", "Alert", props, nil)
	info := extractNodeInfo(node)

	if !info.hidden {
		t.Error("hidden should be true")
	}
	if info.live != "polite" {
		t.Errorf("live = %q, want polite", info.live)
	}
	if !info.modal {
		t.Error("modal should be true")
	}
}

func TestRenderYAML_HiddenLiveModal(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Page", nil, []string{"dlg", "status"}),
		func() *proto.AccessibilityAXNode {
			return makeAXNode("dlg", "dialog", "Confirm", []*proto.AccessibilityAXProperty{
				{Name: "modal", Value: makeAXValue(true)},
			}, []string{"btn"})
		}(),
		makeAXNode("btn", "button", "OK", nil, nil),
		func() *proto.AccessibilityAXNode {
			return makeAXNode("status", "status", "3 new messages", []*proto.AccessibilityAXProperty{
				{Name: "live", Value: makeAXValue("assertive")},
			}, nil)
		}(),
	}

	got := renderAXTreeYAML(nodes, 0)

	checks := []struct{ desc, pattern string }{
		{"modal dialog", `[modal]`},
		{"live region", `[live=assertive]`},
		{"status text", `"3 new messages"`},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.pattern) {
			t.Errorf("%s: expected %q in:\n%s", c.desc, c.pattern, got)
		}
	}
}

func TestRenderYAML_FormControls(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Form Controls", nil, []string{"checkbox1", "radio1", "combo1"}),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("checkbox1", "checkbox", "Subscribe", []*proto.AccessibilityAXProperty{
				{Name: "checked", Value: makeAXValue(true)},
			}, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("radio1", "radio", "Option A", []*proto.AccessibilityAXProperty{
				{Name: "selected", Value: makeAXValue(false)},
			}, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("combo1", "combobox", "Language", []*proto.AccessibilityAXProperty{
				{Name: "expanded", Value: makeAXValue(false)},
			}, nil)
			n.Value = makeAXValue("English")
			return n
		}(),
	}

	got := renderAXTreeYAML(nodes, 0)

	checks := []struct {
		desc, pattern string
	}{
		{"checkbox checked", `- checkbox "Subscribe" [ref=e1] [cursor=pointer] [checked]`},
		{"radio not selected", `- radio "Option A" [ref=e2] [cursor=pointer]`},
		{"combobox with value", `- combobox "Language" [ref=e3] [cursor=pointer] [value="English"]`},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.pattern) {
			t.Errorf("%s: expected %q in output:\n%s", c.desc, c.pattern, got)
		}
	}

	// Verify expanded attribute is NOT present when false
	if strings.Contains(got, "[expanded]") {
		t.Errorf("expanded=false should not produce [expanded], got:\n%s", got)
	}
}

func TestRenderYAML_HeadingHierarchy(t *testing.T) {
	nodes := []*proto.AccessibilityAXNode{
		makeAXNode("root", "RootWebArea", "Document", nil, []string{"h1", "h2", "h3"}),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("h1", "heading", "Main Title", []*proto.AccessibilityAXProperty{
				{Name: "level", Value: makeAXValue(float64(1))},
			}, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("h2", "heading", "Subsection", []*proto.AccessibilityAXProperty{
				{Name: "level", Value: makeAXValue(float64(2))},
			}, nil)
			return n
		}(),
		func() *proto.AccessibilityAXNode {
			n := makeAXNode("h3", "heading", "Detail", []*proto.AccessibilityAXProperty{
				{Name: "level", Value: makeAXValue(float64(3))},
			}, nil)
			return n
		}(),
	}

	got := renderAXTreeYAML(nodes, 0)

	checks := []struct {
		desc, pattern string
	}{
		{"h1 with level=1", `- heading "Main Title" [level=1]`},
		{"h2 with level=2", `- heading "Subsection" [level=2]`},
		{"h3 with level=3", `- heading "Detail" [level=3]`},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.pattern) {
			t.Errorf("%s: expected %q in output:\n%s", c.desc, c.pattern, got)
		}
	}
}
