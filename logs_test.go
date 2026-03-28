package browser

import "testing"

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
