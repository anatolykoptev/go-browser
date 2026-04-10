package browser

import (
	"strings"
	"sync"

	"github.com/go-rod/rod/lib/proto"
)

// RefMap maps snapshot ref identifiers (e.g. "e1") to DOM BackendNodeIDs.
// Built during snapshot rendering, consumed by resolveElement during actions.
// Thread-safe: snapshot and actions may run on different goroutines.
type RefMap struct {
	mu   sync.RWMutex
	refs map[string]proto.DOMBackendNodeID
}

// NewRefMap creates an empty RefMap.
func NewRefMap() *RefMap {
	return &RefMap{refs: make(map[string]proto.DOMBackendNodeID)}
}

// Store records a ref → backendNodeID mapping.
func (rm *RefMap) Store(ref string, id proto.DOMBackendNodeID) {
	rm.mu.Lock()
	rm.refs[ref] = id
	rm.mu.Unlock()
}

// Resolve returns the BackendNodeID for a ref, or false if not found.
func (rm *RefMap) Resolve(ref string) (proto.DOMBackendNodeID, bool) {
	rm.mu.RLock()
	id, ok := rm.refs[ref]
	rm.mu.RUnlock()
	return id, ok
}

// Clear removes all entries (called before each new snapshot).
func (rm *RefMap) Clear() {
	rm.mu.Lock()
	rm.refs = make(map[string]proto.DOMBackendNodeID)
	rm.mu.Unlock()
}

// ParseRef checks if selector starts with "ref=" and returns the ref key.
func ParseRef(selector string) (string, bool) {
	if strings.HasPrefix(selector, "ref=") {
		return strings.TrimPrefix(selector, "ref="), true
	}
	return "", false
}
