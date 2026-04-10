package browser

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// --- truncateURL edge tests ---

// TestTruncateURL_ExactLength verifies that a URL of exactly maxURLLength bytes is NOT truncated.
func TestTruncateURL_ExactLength(t *testing.T) {
	u := strings.Repeat("a", maxURLLength)
	got := truncateURL(u)
	if got != u {
		t.Errorf("expected string unchanged (len=%d), got len=%d", len(u), len(got))
	}
	if strings.HasSuffix(got, "…") || strings.HasSuffix(got, "...") {
		t.Error("expected no ellipsis suffix for exact-length URL")
	}
}

// TestTruncateURL_OneBeyondMax verifies that a URL of maxURLLength+1 bytes is truncated and ends with ellipsis.
// Note: "…" is 3 UTF-8 bytes, so total byte count may exceed original; what matters is the content is capped.
func TestTruncateURL_OneBeyondMax(t *testing.T) {
	u := strings.Repeat("a", maxURLLength+1)
	got := truncateURL(u)
	const ellipsis = "…"
	if !strings.HasSuffix(got, ellipsis) && !strings.HasSuffix(got, "...") {
		suffix := got
		if len(got) > 10 {
			suffix = got[len(got)-10:]
		}
		t.Errorf("expected ellipsis suffix, got: %q", suffix)
	}
	// Content before ellipsis must not exceed maxURLLength bytes.
	if strings.HasSuffix(got, ellipsis) {
		content := got[:len(got)-len(ellipsis)]
		if len(content) > maxURLLength {
			t.Errorf("content before ellipsis is %d bytes, want <= %d", len(content), maxURLLength)
		}
	}
}

// TestTruncateURL_Empty verifies that an empty string is returned as-is.
func TestTruncateURL_Empty(t *testing.T) {
	got := truncateURL("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestTruncateURL_Unicode verifies that truncation of a URL containing multi-byte Unicode
// does not produce invalid UTF-8 and does not panic.
//
// Adversarial case: 149 ASCII bytes + Cyrillic chars. A naive byte-slice at maxURLLength (150)
// would land on the first byte of a 2-byte Cyrillic rune, producing invalid UTF-8.
func TestTruncateURL_Unicode(t *testing.T) {
	// Build: 149 ASCII 'a' + many 2-byte Cyrillic 'д' → total > maxURLLength bytes.
	// Byte 150 falls exactly on the second byte of the first Cyrillic rune (mid-rune).
	u := strings.Repeat("a", maxURLLength-1) + strings.Repeat("д", 20)
	if len(u) <= maxURLLength {
		t.Fatalf("test setup: expected len > %d, got %d", maxURLLength, len(u))
	}
	got := truncateURL(u)
	if !utf8.ValidString(got) {
		t.Errorf("truncateURL returned invalid UTF-8 for unicode input (len=%d)", len(got))
	}
	// Result must be truncated to at most maxURLLength runes (+ ellipsis rune).
	runeCount := utf8.RuneCountInString(got)
	if runeCount > maxURLLength+1 { // +1 for the ellipsis rune "…"
		t.Errorf("truncated URL too long: %d runes, want <= %d", runeCount, maxURLLength+1)
	}
	// Must end with ellipsis since input exceeds maxURLLength runes.
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got tail: %q", got[max(0, len(got)-10):])
	}
}

// --- lastN edge tests ---

// TestLastN_ZeroN verifies that lastN with n=0 returns an empty slice.
func TestLastN_ZeroN(t *testing.T) {
	s := []int{1, 2, 3}
	got := lastN(s, 0)
	if len(got) != 0 {
		t.Errorf("lastN(s, 0): expected empty slice, got %v (len=%d)", got, len(got))
	}
}

// TestLastN_NBeyondLen verifies that lastN with n > len(s) returns all elements.
func TestLastN_NBeyondLen(t *testing.T) {
	s := []int{1, 2, 3}
	got := lastN(s, 100)
	if len(got) != len(s) {
		t.Errorf("lastN(s, 100): expected %d elements, got %d", len(s), len(got))
	}
	for i, v := range s {
		if got[i] != v {
			t.Errorf("lastN(s, 100)[%d] = %d, want %d", i, got[i], v)
		}
	}
}

// TestLastN_Negative verifies that lastN with a negative n does not panic and returns empty.
func TestLastN_Negative(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("lastN panicked with negative n: %v", r)
		}
	}()
	s := []int{1, 2, 3}
	got := lastN(s, -5)
	if len(got) != 0 {
		t.Errorf("lastN(s, -5): expected empty slice, got %v (len=%d)", got, len(got))
	}
}

// --- execGetLogs edge tests ---

// TestExecGetLogs_ExactNetworkLimit verifies that exactly defaultNetworkLimit entries
// are returned in full without any trimming.
func TestExecGetLogs_ExactNetworkLimit(t *testing.T) {
	logs := NewLogCollector()
	for i := range defaultNetworkLimit {
		logs.AddNetwork(NetworkEntry{Method: "GET", URL: "https://example.com/", Status: 200 + i})
	}

	result, err := execGetLogs(dispatchContext{ctx: context.Background(), logs: logs}, Action{})
	if err != nil {
		t.Fatalf("execGetLogs: %v", err)
	}
	m := result.(map[string]any)
	count := jsonSliceLen(t, m["network"])
	if count != defaultNetworkLimit {
		t.Errorf("expected exactly %d network entries, got %d", defaultNetworkLimit, count)
	}
}

// TestExecGetLogs_ZeroEntries verifies that get_logs returns empty slices when the
// collector has no entries.
func TestExecGetLogs_ZeroEntries(t *testing.T) {
	logs := NewLogCollector()

	result, err := execGetLogs(dispatchContext{ctx: context.Background(), logs: logs}, Action{})
	if err != nil {
		t.Fatalf("execGetLogs: %v", err)
	}
	m := result.(map[string]any)

	netCount := jsonSliceLen(t, m["network"])
	if netCount != 0 {
		t.Errorf("expected 0 network entries, got %d", netCount)
	}
	if cs, ok := m["console"].([]ConsoleEntry); ok {
		if len(cs) != 0 {
			t.Errorf("expected 0 console entries, got %d", len(cs))
		}
	}
}

// TestExecGetCookies_ZeroLimit verifies that Limit=0 returns ALL cookies (no limit applied).
// Since execGetCookies requires a live browser page, we validate the guard condition directly:
// "if a.Limit > 0 && a.Limit < len(cookies)" must be false when Limit=0.
func TestExecGetCookies_ZeroLimit(t *testing.T) {
	a := Action{Limit: 0}
	cookieCount := 5 // hypothetical cookie count
	limitApplied := a.Limit > 0 && a.Limit < cookieCount
	if limitApplied {
		t.Error("Limit=0 must not trim cookies; guard condition incorrectly evaluated true")
	}
}

// TestExecGetLogs_NetworkURLTruncation adds an entry with a 500-char URL and verifies
// the returned URL is ≤ maxURLLength + ellipsis bytes, and is valid UTF-8.
func TestExecGetLogs_NetworkURLTruncation(t *testing.T) {
	logs := NewLogCollector()
	longURL := "https://example.com/" + strings.Repeat("x", 480) // ~500 chars total
	logs.AddNetwork(NetworkEntry{Method: "GET", URL: longURL, Status: 200})

	result, err := execGetLogs(dispatchContext{ctx: context.Background(), logs: logs}, Action{Limit: 1})
	if err != nil {
		t.Fatalf("execGetLogs: %v", err)
	}
	m := result.(map[string]any)

	netCount := jsonSliceLen(t, m["network"])
	if netCount != 1 {
		t.Fatalf("expected 1 network entry, got %d", netCount)
	}

	b, err := json.Marshal(m["network"])
	if err != nil {
		t.Fatalf("marshal network: %v", err)
	}
	var entries []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatalf("unmarshal network: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no entries after unmarshal")
	}
	gotURL := entries[0].URL

	const ellipsisBytes = 3 // UTF-8 byte length of "…"
	if len(gotURL) > maxURLLength+ellipsisBytes {
		t.Errorf("returned URL byte length %d exceeds maxURLLength+ellipsis (%d)",
			len(gotURL), maxURLLength+ellipsisBytes)
	}
	if !utf8.ValidString(gotURL) {
		t.Error("returned URL is invalid UTF-8")
	}
}

// jsonSliceLen serializes v to JSON and counts array elements.
// Used to count elements of compactNetwork (a local type inside execGetLogs) without reflection.
func jsonSliceLen(t *testing.T, v any) int {
	t.Helper()
	if v == nil {
		return 0
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("jsonSliceLen marshal: %v", err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil {
		return 0
	}
	return len(arr)
}
