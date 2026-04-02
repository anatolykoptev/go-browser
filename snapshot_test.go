package browser

import (
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
