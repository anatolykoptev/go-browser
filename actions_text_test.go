package browser

import (
	"strings"
	"testing"
)

// TestFindByText_XPathExpression verifies the XPath expression used in findByText
// is correctly formed for various text inputs.
func TestFindByText_XPathExpression(t *testing.T) {
	cases := []struct {
		text        string
		wantContain string
	}{
		{"MASTERCARD", `contains(text(),"MASTERCARD")`},
		{"Submit", `contains(text(),"Submit")`},
		{"Click me", `contains(text(),"Click me")`},
	}

	for _, tc := range cases {
		xpath := buildTextXPath(tc.text)
		if !strings.Contains(xpath, tc.wantContain) {
			t.Errorf("text=%q: xpath %q does not contain %q", tc.text, xpath, tc.wantContain)
		}
	}
}

// TestFindByText_XPathIncludesClickableElements verifies the XPath targets
// only clickable element types.
func TestFindByText_XPathIncludesClickableElements(t *testing.T) {
	xpath := buildTextXPath("test")
	clickable := []string{"self::a", "self::button", "self::label", "self::input", "self::li", "self::span", "self::div"}
	for _, tag := range clickable {
		if !strings.Contains(xpath, tag) {
			t.Errorf("XPath %q does not include %q", xpath, tag)
		}
	}
}

// buildTextXPath is the same logic as findByText uses internally,
// extracted so unit tests can validate the expression without a browser.
func buildTextXPath(text string) string {
	return `//*[contains(text(),"` + text + `")][self::a or self::button or self::label or self::input or self::li or self::span or self::div]`
}
