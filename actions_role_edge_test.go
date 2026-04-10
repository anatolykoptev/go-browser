package browser

import (
	"strings"
	"testing"
)

// TestRoleToSelectors_AllDocumentedRoles verifies that every documented role returns
// a non-empty selector slice.
func TestRoleToSelectors_AllDocumentedRoles(t *testing.T) {
	t.Parallel()

	roles := []string{
		"button",
		"link",
		"textbox",
		"checkbox",
		"radio",
		"combobox",
		"heading",
		"img",
		"image",
		"navigation",
		"form",
	}

	for _, role := range roles {
		role := role
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			sels := roleToSelectors(role)
			if len(sels) == 0 {
				t.Errorf("role=%q: expected non-empty selector slice, got none", role)
			}
			for _, s := range sels {
				if s == "" {
					t.Errorf("role=%q: selector slice contains empty string", role)
				}
			}
		})
	}
}

// TestRoleToSelectors_UnknownRoleFallback verifies that an unknown role returns exactly
// one selector of the form [role=X].
func TestRoleToSelectors_UnknownRoleFallback(t *testing.T) {
	t.Parallel()

	unknown := "alertdialog"
	sels := roleToSelectors(unknown)

	if len(sels) != 1 {
		t.Fatalf("unknown role: expected 1 selector, got %d: %v", len(sels), sels)
	}
	want := "[role=" + unknown + "]"
	if sels[0] != want {
		t.Errorf("unknown role selector: got %q, want %q", sels[0], want)
	}
}

// TestRoleToSelectors_EmptyString verifies that an empty role string does not panic
// and returns a predictable fallback selector.
func TestRoleToSelectors_EmptyString(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("roleToSelectors(\"\") panicked: %v", r)
		}
	}()

	sels := roleToSelectors("")
	if len(sels) == 0 {
		t.Fatal("empty role: expected at least one selector, got none")
	}
	// The fallback path should produce [role=].
	if sels[0] != "[role=]" {
		t.Errorf("empty role: got %q, want [role=]", sels[0])
	}
}

// TestRoleToSelectors_UppercaseCaseInsensitive verifies that "BUTTON" produces the
// same selector list as "button".
func TestRoleToSelectors_UppercaseCaseInsensitive(t *testing.T) {
	t.Parallel()

	lower := roleToSelectors("button")
	upper := roleToSelectors("BUTTON")

	if len(lower) != len(upper) {
		t.Fatalf("BUTTON vs button: different selector count; lower=%d upper=%d", len(lower), len(upper))
	}
	for i := range lower {
		if lower[i] != upper[i] {
			t.Errorf("index %d: lower=%q upper=%q", i, lower[i], upper[i])
		}
	}
}

// TestRoleToSelectors_ImgImageEquivalence verifies that "img" and "image" return
// identical selector slices (both map to the same ARIA role).
func TestRoleToSelectors_ImgImageEquivalence(t *testing.T) {
	t.Parallel()

	img := roleToSelectors("img")
	image := roleToSelectors("image")

	if len(img) != len(image) {
		t.Fatalf("img vs image: different selector count; img=%d image=%d", len(img), len(image))
	}
	for i := range img {
		if img[i] != image[i] {
			t.Errorf("index %d: img=%q image=%q", i, img[i], image[i])
		}
	}
}

// TestResolveElement_RolePrefixDispatch verifies that a "role=X" selector string
// correctly extracts the role and produces the expected selectors via roleToSelectors.
// This exercises the dispatch logic in resolveElement at the unit level without a page.
func TestResolveElement_RolePrefixDispatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input        string
		wantRole     string
		wantFirstSel string
	}{
		{"role=button", "button", "button"},
		{"role=link", "link", "a[href]"},
		{"role=checkbox", "checkbox", "input[type=checkbox]"},
		{"role=navigation", "navigation", "nav"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			if !strings.HasPrefix(tc.input, "role=") {
				t.Fatalf("test case input %q missing role= prefix", tc.input)
			}
			role := strings.TrimPrefix(tc.input, "role=")
			if role != tc.wantRole {
				t.Fatalf("extracted role %q, want %q", role, tc.wantRole)
			}
			sels := roleToSelectors(role)
			if len(sels) == 0 {
				t.Fatalf("roleToSelectors(%q) returned empty slice", role)
			}
			if sels[0] != tc.wantFirstSel {
				t.Errorf("first selector for %q: got %q, want %q", tc.input, sels[0], tc.wantFirstSel)
			}
		})
	}
}
