package browser

import (
	"context"
	"testing"
	"time"
)

func TestRefWorkflow_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires Chrome")
	}

	// Use the shared browser (launched by acquireSharedBrowser or connected
	// to CLOAKBROWSER_WS_URL). NewChromeManager("") fails when no Chrome is
	// already listening on a default port — in CI we launch Chrome via the
	// shared launcher, so build the ChromeManager from that browser instance.
	b := acquireSharedBrowser(t)

	guard, err := installEgressGuard(b)
	if err != nil {
		t.Fatalf("install egress guard: %v", err)
	}
	chrome := &ChromeManager{
		browser: b,
		pool:    NewContextPool(b),
		guard:   guard,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp := RunInteract(ctx, chrome, InteractRequest{
		URL:       "https://example.com",
		NoStealth: true,
		Actions: []Action{
			{Type: "snapshot", Filter: "interactive"},
			{Type: "click", Selector: "ref=e1"},
		},
	})

	if resp.Status != "ok" {
		t.Fatalf("status=%s, error=%s", resp.Status, resp.Error)
	}
	if len(resp.Actions) != 2 {
		t.Fatalf("expected 2 action results, got %d", len(resp.Actions))
	}
	if !resp.Actions[0].Ok {
		t.Fatalf("snapshot failed: %s", resp.Actions[0].Error)
	}
	if !resp.Actions[1].Ok {
		t.Fatalf("ref click failed: %s", resp.Actions[1].Error)
	}
}
