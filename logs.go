package browser

import (
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const maxLogEntries = 1000

// NetworkEntry is a captured network request/response.
type NetworkEntry struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ConsoleEntry is a captured console API call.
type ConsoleEntry struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

// LogCollector accumulates network and console entries from CDP events.
type LogCollector struct {
	mu      sync.Mutex
	network []NetworkEntry
	console []ConsoleEntry
}

// NewLogCollector creates an empty log collector.
func NewLogCollector() *LogCollector {
	return &LogCollector{
		network: make([]NetworkEntry, 0, 64),
		console: make([]ConsoleEntry, 0, 64),
	}
}

// AddNetwork appends a network entry (capped at maxLogEntries).
func (c *LogCollector) AddNetwork(e NetworkEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.network) < maxLogEntries {
		c.network = append(c.network, e)
	}
}

// AddConsole appends a console entry (capped at maxLogEntries).
func (c *LogCollector) AddConsole(e ConsoleEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.console) < maxLogEntries {
		c.console = append(c.console, e)
	}
}

// Collect returns all accumulated entries.
func (c *LogCollector) Collect() ([]NetworkEntry, []ConsoleEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]NetworkEntry{}, c.network...), append([]ConsoleEntry{}, c.console...)
}

// SubscribeCDP enables Network and Runtime domains and starts collecting events.
// Call this after page creation and before navigation.
// The event listener runs in a background goroutine until the page is closed.
func (c *LogCollector) SubscribeCDP(page *rod.Page) {
	_ = proto.NetworkEnable{}.Call(page)
	_ = proto.RuntimeEnable{}.Call(page)

	go page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			c.AddNetwork(NetworkEntry{
				Method: e.Request.Method,
				URL:    e.Request.URL,
			})
		},
		func(e *proto.NetworkResponseReceived) {
			c.mu.Lock()
			defer c.mu.Unlock()
			for i := len(c.network) - 1; i >= 0; i-- {
				if c.network[i].URL == e.Response.URL && c.network[i].Status == 0 {
					c.network[i].Status = e.Response.Status
					break
				}
			}
		},
		func(e *proto.RuntimeConsoleAPICalled) {
			var parts []string
			for _, arg := range e.Args {
				if arg.Value.Val() != nil {
					parts = append(parts, arg.Value.Str())
				}
			}
			c.AddConsole(ConsoleEntry{
				Level: string(e.Type),
				Text:  strings.Join(parts, " "),
			})
		},
	)()
}
