package browser

import (
	"testing"
)

// TestStealthLogCollector_SkipsRuntimeEnable verifies that a stealth LogCollector
// does not call Runtime.enable or Console.enable (the #1 CDP detection vector).
// We can't easily test the actual CDP calls without a browser, but we can verify
// that stealthMode is set correctly and SubscribeConsole is a no-op.
func TestStealthLogCollector_SkipsRuntimeEnable(t *testing.T) {
	c := NewStealthLogCollector()
	if !c.stealthMode {
		t.Fatal("NewStealthLogCollector: stealthMode should be true")
	}

	// SubscribeConsole should be a no-op in stealth mode (no panic, no error)
	// Passing nil page is safe because stealthMode returns before touching page.
	c.SubscribeConsole(nil)

	// Regular LogCollector should NOT have stealthMode
	regular := NewLogCollector()
	if regular.stealthMode {
		t.Fatal("NewLogCollector: stealthMode should be false")
	}
}

// TestStealthLogCollector_StillCollectsNetwork verifies that stealth mode
// doesn't affect the ring buffer / collection logic — only CDP domain enables.
func TestStealthLogCollector_StillCollectsNetwork(t *testing.T) {
	c := NewStealthLogCollector()
	c.AddNetwork(NetworkEntry{Method: "GET", URL: "https://example.com", Status: 200})
	c.AddNavigation(NavigationEntry{URL: "https://example.com"})

	net, _ := c.Collect()
	if len(net) != 1 {
		t.Errorf("expected 1 network entry, got %d", len(net))
	}
}
