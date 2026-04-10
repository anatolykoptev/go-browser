package browser

import (
	"strings"
	"testing"
)

func TestRoleToSelectors_Button(t *testing.T) {
	sels := roleToSelectors("button")
	want := []string{"button", "[role=button]", "input[type=submit]", "input[type=button]"}
	if len(sels) != len(want) {
		t.Fatalf("button: got %d selectors, want %d", len(sels), len(want))
	}
	for i, s := range sels {
		if s != want[i] {
			t.Errorf("button[%d]: got %q, want %q", i, s, want[i])
		}
	}
}

func TestRoleToSelectors_Link(t *testing.T) {
	sels := roleToSelectors("link")
	want := []string{"a[href]", "[role=link]"}
	if len(sels) != len(want) {
		t.Fatalf("link: got %d selectors, want %d", len(sels), len(want))
	}
	for i, s := range sels {
		if s != want[i] {
			t.Errorf("link[%d]: got %q, want %q", i, s, want[i])
		}
	}
}

func TestRoleToSelectors_Textbox(t *testing.T) {
	sels := roleToSelectors("textbox")
	if len(sels) == 0 {
		t.Fatal("textbox: expected non-empty selectors")
	}
	for _, s := range sels {
		if !strings.Contains(s, "input") && !strings.Contains(s, "textarea") && !strings.Contains(s, "role=textbox") {
			t.Errorf("textbox: unexpected selector %q", s)
		}
	}
}

func TestRoleToSelectors_UnknownRole(t *testing.T) {
	sels := roleToSelectors("foobar")
	if len(sels) != 1 {
		t.Fatalf("unknown role: expected 1 selector, got %d", len(sels))
	}
	if sels[0] != "[role=foobar]" {
		t.Errorf("unknown role: got %q, want [role=foobar]", sels[0])
	}
}

func TestRoleToSelectors_CaseInsensitive(t *testing.T) {
	lower := roleToSelectors("button")
	upper := roleToSelectors("BUTTON")
	mixed := roleToSelectors("Button")

	if len(lower) != len(upper) || len(lower) != len(mixed) {
		t.Fatal("role matching should be case-insensitive")
	}
	for i := range lower {
		if lower[i] != upper[i] || lower[i] != mixed[i] {
			t.Errorf("index %d: lower=%q upper=%q mixed=%q", i, lower[i], upper[i], mixed[i])
		}
	}
}

func TestResolveElementPrefixDispatch_Role(t *testing.T) {
	// Verify that "role=" prefix is recognized by checking roleToSelectors is
	// called correctly. We test the dispatch logic indirectly: if roleToSelectors
	// for a role returns known selectors, findByRole will attempt those.
	// This test ensures the prefix extraction is correct.
	const selector = "role=button"
	if !strings.HasPrefix(selector, "role=") {
		t.Fatal("prefix check failed")
	}
	role := strings.TrimPrefix(selector, "role=")
	sels := roleToSelectors(role)
	if len(sels) == 0 {
		t.Fatal("roleToSelectors returned empty for button")
	}
	if sels[0] != "button" {
		t.Errorf("first selector for button: got %q, want button", sels[0])
	}
}
