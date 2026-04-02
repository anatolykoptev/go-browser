package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// collectLinkURLs fetches the full DOM tree and extracts href attributes
// from <a> elements, returning a map from BackendNodeID to href.
func collectLinkURLs(page *rod.Page) map[proto.DOMBackendNodeID]string {
	depth := -1
	res, err := proto.DOMGetDocument{Depth: &depth, Pierce: true}.Call(page)
	if err != nil {
		return nil
	}
	urls := make(map[proto.DOMBackendNodeID]string)
	walkDOM(res.Root, urls)
	return urls
}

// walkDOM recursively walks a DOMNode tree, collecting href attributes
// from anchor elements into the urls map.
func walkDOM(node *proto.DOMNode, urls map[proto.DOMBackendNodeID]string) {
	if node == nil {
		return
	}
	if node.NodeName == "A" || node.NodeName == "a" {
		attrs := node.Attributes
		for i := 0; i+1 < len(attrs); i += 2 {
			if attrs[i] == "href" {
				urls[node.BackendNodeID] = attrs[i+1]
				break
			}
		}
	}
	for _, child := range node.Children {
		walkDOM(child, urls)
	}
	if node.ContentDocument != nil {
		walkDOM(node.ContentDocument, urls)
	}
	for _, sr := range node.ShadowRoots {
		walkDOM(sr, urls)
	}
}

// applyLinkURLs maps AX link nodes to their DOM BackendNodeID, then sets
// the url field from the collected href map.
func applyLinkURLs(
	index map[string]*nodeInfo,
	nodes []*proto.AccessibilityAXNode,
	urls map[proto.DOMBackendNodeID]string,
) {
	// Build axNodeID → BackendDOMNodeID map.
	backendIDs := make(map[string]proto.DOMBackendNodeID, len(nodes))
	for _, node := range nodes {
		if node.BackendDOMNodeID != 0 {
			backendIDs[string(node.NodeID)] = node.BackendDOMNodeID
		}
	}
	// Set url on link nodes found in index.
	for id, info := range index {
		if info.role != "link" {
			continue
		}
		bid, ok := backendIDs[id]
		if !ok {
			continue
		}
		if href, found := urls[bid]; found {
			info.url = href
		}
	}
}
