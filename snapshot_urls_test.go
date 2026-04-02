package browser

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

func TestWalkDOM_CollectsAnchorHrefs(t *testing.T) {
	root := &proto.DOMNode{
		NodeName:      "HTML",
		BackendNodeID: 1,
		Children: []*proto.DOMNode{
			{
				NodeName:      "BODY",
				BackendNodeID: 2,
				Children: []*proto.DOMNode{
					{
						NodeName:      "A",
						BackendNodeID: 10,
						Attributes:    []string{"href", "/home", "class", "nav-link"},
					},
					{
						NodeName:      "DIV",
						BackendNodeID: 3,
						Children: []*proto.DOMNode{
							{
								NodeName:      "A",
								BackendNodeID: 11,
								Attributes:    []string{"href", "https://example.com/settings"},
							},
						},
					},
					{
						NodeName:      "A",
						BackendNodeID: 12,
						Attributes:    []string{"class", "no-href"},
					},
				},
			},
		},
	}

	urls := make(map[proto.DOMBackendNodeID]string)
	walkDOM(root, urls)

	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
	if urls[10] != "/home" {
		t.Errorf("node 10: got %q, want %q", urls[10], "/home")
	}
	if urls[11] != "https://example.com/settings" {
		t.Errorf("node 11: got %q, want %q", urls[11], "https://example.com/settings")
	}
}

func TestWalkDOM_ShadowRootsAndIframes(t *testing.T) {
	root := &proto.DOMNode{
		NodeName:      "HTML",
		BackendNodeID: 1,
		ShadowRoots: []*proto.DOMNode{
			{
				NodeName:      "#shadow-root",
				BackendNodeID: 2,
				Children: []*proto.DOMNode{
					{
						NodeName:      "A",
						BackendNodeID: 20,
						Attributes:    []string{"href", "/shadow"},
					},
				},
			},
		},
		Children: []*proto.DOMNode{
			{
				NodeName:      "IFRAME",
				BackendNodeID: 3,
				ContentDocument: &proto.DOMNode{
					NodeName:      "#document",
					BackendNodeID: 4,
					Children: []*proto.DOMNode{
						{
							NodeName:      "A",
							BackendNodeID: 30,
							Attributes:    []string{"href", "/iframe-link"},
						},
					},
				},
			},
		},
	}

	urls := make(map[proto.DOMBackendNodeID]string)
	walkDOM(root, urls)

	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
	if urls[20] != "/shadow" {
		t.Errorf("shadow node 20: got %q, want %q", urls[20], "/shadow")
	}
	if urls[30] != "/iframe-link" {
		t.Errorf("iframe node 30: got %q, want %q", urls[30], "/iframe-link")
	}
}

func TestWalkDOM_NilNode(t *testing.T) {
	urls := make(map[proto.DOMBackendNodeID]string)
	walkDOM(nil, urls)
	if len(urls) != 0 {
		t.Errorf("expected empty map for nil node, got %v", urls)
	}
}

func TestApplyLinkURLs(t *testing.T) {
	// Set up AX nodes with BackendDOMNodeID.
	axNodes := []*proto.AccessibilityAXNode{
		{
			NodeID:           "ax1",
			BackendDOMNodeID: 10,
		},
		{
			NodeID:           "ax2",
			BackendDOMNodeID: 11,
		},
		{
			NodeID:           "ax3",
			BackendDOMNodeID: 12,
		},
	}

	index := map[string]*nodeInfo{
		"ax1": {role: "link", name: "Home"},
		"ax2": {role: "link", name: "Settings"},
		"ax3": {role: "button", name: "Submit"},
	}

	urls := map[proto.DOMBackendNodeID]string{
		10: "/home",
		11: "/settings",
		12: "/submit", // button, should not get url
	}

	applyLinkURLs(index, axNodes, urls)

	if index["ax1"].url != "/home" {
		t.Errorf("ax1 url = %q, want %q", index["ax1"].url, "/home")
	}
	if index["ax2"].url != "/settings" {
		t.Errorf("ax2 url = %q, want %q", index["ax2"].url, "/settings")
	}
	if index["ax3"].url != "" {
		t.Errorf("ax3 (button) should have no url, got %q", index["ax3"].url)
	}
}

func TestApplyLinkURLs_MissingBackendID(t *testing.T) {
	axNodes := []*proto.AccessibilityAXNode{
		{NodeID: "ax1", BackendDOMNodeID: 0}, // no backend ID
	}
	index := map[string]*nodeInfo{
		"ax1": {role: "link", name: "Orphan"},
	}
	urls := map[proto.DOMBackendNodeID]string{
		99: "/somewhere",
	}

	applyLinkURLs(index, axNodes, urls)

	if index["ax1"].url != "" {
		t.Errorf("link without backend ID should have no url, got %q", index["ax1"].url)
	}
}
