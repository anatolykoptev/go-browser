package browser

import (
	"strings"
	"testing"
)

func TestTruncateURL_Short(t *testing.T) {
	u := "https://example.com/path"
	if got := truncateURL(u); got != u {
		t.Errorf("truncateURL(%q) = %q, want unchanged", u, got)
	}
}

func TestTruncateURL_Long(t *testing.T) {
	u := "https://example.com/" + strings.Repeat("x", 200)
	got := truncateURL(u)
	if len([]rune(got)) > maxURLLength+1 { // +1 for ellipsis rune
		t.Errorf("truncateURL result too long: %d chars", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncateURL should end with ellipsis, got %q", got[len(got)-10:])
	}
}

func TestLastN_LessThanSlice(t *testing.T) {
	s := []int{1, 2, 3, 4, 5}
	got := lastN(s, 3)
	if len(got) != 3 {
		t.Errorf("lastN: got %d elements, want 3", len(got))
	}
	if got[0] != 3 || got[2] != 5 {
		t.Errorf("lastN: got %v, want [3 4 5]", got)
	}
}

func TestLastN_MoreThanSlice(t *testing.T) {
	s := []int{1, 2}
	got := lastN(s, 10)
	if len(got) != 2 {
		t.Errorf("lastN: got %d elements, want 2 (unchanged)", len(got))
	}
}

func TestLastN_Zero(t *testing.T) {
	s := []int{1, 2, 3}
	got := lastN(s, 0)
	if len(got) != 3 {
		t.Errorf("lastN(0): got %d elements, want 3 (unchanged)", len(got))
	}
}

func TestLogCollector_AppendNetwork(t *testing.T) {
	c := NewLogCollector()
	c.AddNetwork(NetworkEntry{Method: "GET", URL: "https://example.com", Status: 200})
	c.AddNetwork(NetworkEntry{Method: "POST", URL: "https://api.example.com", Status: 201})
	net, con := c.Collect()
	if len(net) != 2 {
		t.Errorf("got %d network entries, want 2", len(net))
	}
	if len(con) != 0 {
		t.Errorf("got %d console entries, want 0", len(con))
	}
}

func TestLogCollector_AppendConsole(t *testing.T) {
	c := NewLogCollector()
	c.AddConsole(ConsoleEntry{Level: "log", Text: "hello"})
	_, con := c.Collect()
	if len(con) != 1 {
		t.Errorf("got %d console entries, want 1", len(con))
	}
	if con[0].Text != "hello" {
		t.Errorf("got %q, want %q", con[0].Text, "hello")
	}
}

func TestLogCollector_MaxEntries(t *testing.T) {
	c := NewLogCollector()
	for i := range maxLogEntries + 100 {
		c.AddNetwork(NetworkEntry{URL: "https://example.com/" + string(rune('a'+i%26))})
	}
	net, _ := c.Collect()
	if len(net) != maxLogEntries {
		t.Errorf("got %d entries, want max %d", len(net), maxLogEntries)
	}
}
