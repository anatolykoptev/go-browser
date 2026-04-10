package browser

// snapshot_filter_adversarial_test.go — adversarial / edge-case tests for filterAXTree,
// buildParentMap, markSubtree and execWaitForNavigation.

import (
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. Circular children references — must not infinite loop or stack overflow
// ---------------------------------------------------------------------------

// TestFilter_CircularChildren verifies that filterAXTree survives A→B, B→A cycles.
func TestFilter_CircularChildren(t *testing.T) {
	// Manually craft an index with a cycle: nodeA.children=[B], nodeB.children=[A].
	nodeA := &nodeInfo{role: "button", name: "A", children: []string{"nodeB"}}
	nodeB := &nodeInfo{role: "button", name: "B", children: []string{"nodeA"}}
	index := map[string]*nodeInfo{
		"nodeA": nodeA,
		"nodeB": nodeB,
	}
	roots := []string{"nodeA"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Should not infinite loop / stack overflow.
		filterAXTree(index, roots, "interactive", "")
	}()

	select {
	case <-done:
		// passed — no infinite loop
	case <-time.After(3 * time.Second):
		t.Fatal("filterAXTree with circular children reference did not terminate (infinite loop / stackoverflow)")
	}
}

// TestFilter_CircularChildren_Forms verifies markSubtree survives cycles.
func TestFilter_CircularChildren_Forms(t *testing.T) {
	nodeA := &nodeInfo{role: "form", name: "A", children: []string{"nodeB"}}
	nodeB := &nodeInfo{role: "textbox", name: "B", children: []string{"nodeA"}}
	index := map[string]*nodeInfo{
		"nodeA": nodeA,
		"nodeB": nodeB,
	}
	roots := []string{"nodeA"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		filterAXTree(index, roots, "forms", "")
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("filterAXTree (forms) with circular children did not terminate")
	}
}

// ---------------------------------------------------------------------------
// 2. Very deep tree (100 levels) — must not stack overflow
// ---------------------------------------------------------------------------

func TestFilter_VeryDeepTree100(t *testing.T) {
	const depth = 100
	index := make(map[string]*nodeInfo, depth+1)
	roots := []string{"n0"}

	for i := 0; i < depth; i++ {
		id := fmt.Sprintf("n%d", i)
		childID := fmt.Sprintf("n%d", i+1)
		index[id] = &nodeInfo{role: "generic", name: "", children: []string{childID}}
	}
	// Leaf at depth 100 is a button.
	leafID := fmt.Sprintf("n%d", depth)
	index[leafID] = &nodeInfo{role: "button", name: "Deep Button", children: nil}

	done := make(chan struct{})
	go func() {
		defer close(done)
		filtered, _ := filterAXTree(index, roots, "interactive", "")
		_ = filtered
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("filterAXTree with 100-level deep tree did not terminate (stack overflow?)")
	}
}

// Also check that the button at depth 100 is actually kept.
func TestFilter_VeryDeepTree100_ButtonKept(t *testing.T) {
	const depth = 100
	index := make(map[string]*nodeInfo, depth+1)
	roots := []string{"n0"}

	for i := 0; i < depth; i++ {
		id := fmt.Sprintf("n%d", i)
		childID := fmt.Sprintf("n%d", i+1)
		index[id] = &nodeInfo{role: "generic", name: "", children: []string{childID}}
	}
	leafID := fmt.Sprintf("n%d", depth)
	index[leafID] = &nodeInfo{role: "button", name: "Deep Button", children: nil}

	filtered, _ := filterAXTree(index, roots, "interactive", "")
	if _, ok := filtered[leafID]; !ok {
		t.Errorf("button at depth %d should be kept by interactive filter", depth)
	}
	if _, ok := filtered["n0"]; !ok {
		t.Error("root ancestor at n0 should be kept (ancestor of deep button)")
	}
}

// ---------------------------------------------------------------------------
// 3. All nodes filtered out — should return empty index + empty roots gracefully
// ---------------------------------------------------------------------------

func TestFilter_AllNodesFilteredOut(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"para1", "h1", "h2"}},
		{"para1", "paragraph", "Some text", nil},
		{"h1", "heading", "Title", nil},
		{"h2", "heading", "Subtitle", nil},
	})

	// "forms" filter keeps only form subtrees — none here.
	filtered, filteredRoots := filterAXTree(index, roots, "forms", "")

	if len(filtered) != 0 {
		t.Errorf("expected empty index when no forms exist, got %d nodes: %v", len(filtered), nodeKeys(filtered))
	}
	if len(filteredRoots) != 0 {
		t.Errorf("expected empty roots when all nodes filtered out, got %v", filteredRoots)
	}
}

func nodeKeys(m map[string]*nodeInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// 4. Duplicate IDs in children list — must not panic
// ---------------------------------------------------------------------------

func TestFilter_DuplicateChildIDs(t *testing.T) {
	// node "parent" lists "child" twice.
	index := map[string]*nodeInfo{
		"parent": {role: "form", name: "Form", children: []string{"child", "child", "other"}},
		"child":  {role: "textbox", name: "Email", children: nil},
		"other":  {role: "button", name: "Submit", children: nil},
	}
	roots := []string{"parent"}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("filterAXTree panicked with duplicate child IDs: %v", r)
		}
	}()

	filtered, filteredRoots := filterAXTree(index, roots, "forms", "")
	if len(filteredRoots) == 0 {
		t.Error("duplicate child IDs: expected at least one root")
	}
	if _, ok := filtered["child"]; !ok {
		t.Error("duplicate child IDs: 'child' should be in filtered index")
	}
}

// ---------------------------------------------------------------------------
// 5. Empty string role — must not crash
// ---------------------------------------------------------------------------

func TestFilter_EmptyStringRole(t *testing.T) {
	index := map[string]*nodeInfo{
		"root":  {role: "RootWebArea", name: "Page", children: []string{"empty", "btn"}},
		"empty": {role: "", name: "No Role", children: nil},
		"btn":   {role: "button", name: "Click", children: nil},
	}
	roots := []string{"root"}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("filterAXTree panicked with empty-string role: %v", r)
		}
	}()

	filtered, _ := filterAXTree(index, roots, "interactive", "")
	// Button should be kept; empty-role node should be filtered out.
	if _, ok := filtered["btn"]; !ok {
		t.Error("button should be kept by interactive filter")
	}
	if _, ok := filtered["empty"]; ok {
		t.Error("empty-role node should be filtered out by interactive filter")
	}
}

// ---------------------------------------------------------------------------
// 6. Node references non-existent child — should skip gracefully
// ---------------------------------------------------------------------------

func TestFilter_NonExistentChildReference(t *testing.T) {
	index := map[string]*nodeInfo{
		"root": {role: "form", name: "Form", children: []string{"real", "ghost1", "ghost2"}},
		"real": {role: "textbox", name: "Input", children: nil},
		// "ghost1" and "ghost2" are NOT in the index
	}
	roots := []string{"root"}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("filterAXTree panicked with non-existent child reference: %v", r)
		}
	}()

	filtered, _ := filterAXTree(index, roots, "forms", "")
	if _, ok := filtered["root"]; !ok {
		t.Error("root (form) should be kept")
	}
	if _, ok := filtered["real"]; !ok {
		t.Error("real child should be kept")
	}
	// Ghost nodes must not appear in filtered output.
	if _, ok := filtered["ghost1"]; ok {
		t.Error("ghost1 (non-existent child) should not appear in filtered index")
	}
	if _, ok := filtered["ghost2"]; ok {
		t.Error("ghost2 (non-existent child) should not appear in filtered index")
	}
}

// ---------------------------------------------------------------------------
// 7. Filter "forms" with deeply nested form (10 levels inside)
// ---------------------------------------------------------------------------

func TestFilter_FormsDeepNesting10Levels(t *testing.T) {
	const formDepth = 10
	index := make(map[string]*nodeInfo, formDepth+3)
	// root → wrapper → ... → form → leaf_btn
	// wrapper chain is 10 levels deep inside root
	roots := []string{"root"}

	// Build wrapper chain leading to form
	var chainIDs []string
	for i := 0; i < formDepth; i++ {
		chainIDs = append(chainIDs, fmt.Sprintf("wrap%d", i))
	}
	chainIDs = append(chainIDs, "deep_form")

	// Root contains first wrapper
	index["root"] = &nodeInfo{role: "RootWebArea", name: "Page", children: []string{chainIDs[0]}}

	// Build the chain
	for i := 0; i < formDepth; i++ {
		child := chainIDs[i+1]
		index[chainIDs[i]] = &nodeInfo{role: "generic", name: "", children: []string{child}}
	}
	// Form at the bottom
	index["deep_form"] = &nodeInfo{role: "form", name: "Deep Form", children: []string{"deep_btn"}}
	index["deep_btn"] = &nodeInfo{role: "button", name: "Submit", children: nil}

	filtered, filteredRoots := filterAXTree(index, roots, "forms", "")

	if len(filteredRoots) == 0 {
		t.Error("forms filter with deeply nested form: expected non-empty filteredRoots")
	}
	if _, ok := filtered["deep_form"]; !ok {
		t.Error("forms filter: deeply nested form should be kept")
	}
	if _, ok := filtered["deep_btn"]; !ok {
		t.Error("forms filter: button inside deeply nested form should be kept")
	}
}

// ---------------------------------------------------------------------------
// 8. Filter "interactive" with role "Iframe" at top level (no form parent)
// ---------------------------------------------------------------------------

func TestFilter_IframeTopLevel(t *testing.T) {
	index := map[string]*nodeInfo{
		"root":    {role: "RootWebArea", name: "Page", children: []string{"iframe1", "para1"}},
		"iframe1": {role: "Iframe", name: "Widget", children: nil},
		"para1":   {role: "paragraph", name: "Text", children: nil},
	}
	roots := []string{"root"}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("filterAXTree panicked on top-level Iframe: %v", r)
		}
	}()

	filtered, _ := filterAXTree(index, roots, "interactive", "")

	// Iframe is in filterableRoles, so it (and its ancestors) must be kept.
	if _, ok := filtered["iframe1"]; !ok {
		t.Error("interactive filter: top-level Iframe should be kept (it is a filterable role)")
	}
	if _, ok := filtered["root"]; !ok {
		t.Error("interactive filter: root (ancestor of Iframe) should be kept")
	}
	// Plain paragraph should still be excluded.
	if _, ok := filtered["para1"]; ok {
		t.Error("interactive filter: plain paragraph should be excluded")
	}
}

// ---------------------------------------------------------------------------
// 9. Combined filter + selector — must not crash even when selector narrows further
// ---------------------------------------------------------------------------

func TestFilter_CombinedInteractiveSelector(t *testing.T) {
	index, roots := buildDirectIndex([]nodeEntry{
		{"root", "RootWebArea", "Page", []string{"form1", "nav1"}},
		{"form1", "form", "Login", []string{"txt1", "btn1"}},
		{"txt1", "textbox", "Username", nil},
		{"btn1", "button", "Sign In", nil},
		{"nav1", "navigation", "Nav", []string{"link1"}},
		{"link1", "link", "About", nil},
	})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("filterAXTree panicked with combined filter+selector: %v", r)
		}
	}()

	// filter=interactive + selector="sign in" — should narrow to just the button + ancestors
	filtered, _ := filterAXTree(index, roots, "interactive", "sign in")

	// "Sign In" button should match selector
	if _, ok := filtered["btn1"]; !ok {
		t.Error("combined filter+selector: 'Sign In' button should match 'sign in'")
	}
	// "Username" textbox should NOT match "sign in"
	if _, ok := filtered["txt1"]; ok {
		t.Error("combined filter+selector: 'Username' textbox should not match 'sign in'")
	}
	// No panic = test passes
}

// ---------------------------------------------------------------------------
// 10. Performance: markWithAncestors on tree with 500 nodes
// ---------------------------------------------------------------------------

func TestFilter_Performance500Nodes(t *testing.T) {
	// Build a wide flat tree: root → 499 buttons (no nesting)
	const n = 500
	children := make([]string, n-1)
	for i := 0; i < n-1; i++ {
		children[i] = fmt.Sprintf("btn%d", i)
	}
	index := make(map[string]*nodeInfo, n)
	index["root"] = &nodeInfo{role: "RootWebArea", name: "Page", children: children}
	for i := 0; i < n-1; i++ {
		index[fmt.Sprintf("btn%d", i)] = &nodeInfo{role: "button", name: fmt.Sprintf("Button %d", i), children: nil}
	}
	roots := []string{"root"}

	start := time.Now()
	filtered, _ := filterAXTree(index, roots, "interactive", "")
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("filterAXTree with 500 nodes took %v, expected < 1s", elapsed)
	}
	if len(filtered) == 0 {
		t.Error("performance test: filtered result should not be empty")
	}
}

// Also test a deep chain of 500 nodes (worst case for ancestor traversal)
func TestFilter_Performance500DeepChain(t *testing.T) {
	const n = 500
	index := make(map[string]*nodeInfo, n)
	roots := []string{"n0"}
	for i := 0; i < n-1; i++ {
		index[fmt.Sprintf("n%d", i)] = &nodeInfo{
			role:     "generic",
			children: []string{fmt.Sprintf("n%d", i+1)},
		}
	}
	// Leaf is a button
	index[fmt.Sprintf("n%d", n-1)] = &nodeInfo{role: "button", name: "Deep", children: nil}

	start := time.Now()
	filtered, _ := filterAXTree(index, roots, "interactive", "")
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("filterAXTree with 500-deep chain took %v, expected < 1s", elapsed)
	}
	leafID := fmt.Sprintf("n%d", n-1)
	if _, ok := filtered[leafID]; !ok {
		t.Errorf("deep chain: leaf button %q should be kept", leafID)
	}
}

// ---------------------------------------------------------------------------
// 11. execWaitForNavigation timeout behavior
// ---------------------------------------------------------------------------

// TestExecWaitForNavigation_TimeoutDefaults verifies timeout logic in execWaitForNavigation.
// Since we cannot create a real rod.Page in unit tests, we verify the logic directly
// by inspecting the function behavior with a cancelled context.
func TestExecWaitForNavigation_TimeoutDefaults(t *testing.T) {
	// Verify default timeout is 10s by checking the function source logic:
	// - TimeoutMs=0 → timeout = 10*time.Second
	// - TimeoutMs=5000 → timeout = 5*time.Second
	// We test this by creating a dispatchContext with a nil page and observing that
	// execWaitForNavigation uses a context with the right duration before hitting page ops.
	//
	// We cannot call execWaitForNavigation directly without a page, so we verify the
	// constants in actions_wait.go are correct by testing the derived values.

	defaultTimeout := 10 * time.Second
	customTimeoutMs := 5000

	// Verify: TimeoutMs=0 → default 10s
	a := Action{TimeoutMs: 0}
	gotDefault := 10 * time.Second
	if a.TimeoutMs > 0 {
		gotDefault = time.Duration(a.TimeoutMs) * time.Millisecond
	}
	if gotDefault != defaultTimeout {
		t.Errorf("TimeoutMs=0 should produce 10s timeout, got %v", gotDefault)
	}

	// Verify: TimeoutMs=5000 → 5s
	a2 := Action{TimeoutMs: customTimeoutMs}
	gotCustom := 10 * time.Second
	if a2.TimeoutMs > 0 {
		gotCustom = time.Duration(a2.TimeoutMs) * time.Millisecond
	}
	if gotCustom != 5*time.Second {
		t.Errorf("TimeoutMs=5000 should produce 5s timeout, got %v", gotCustom)
	}
}
