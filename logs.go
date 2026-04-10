package browser

import (
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const (
	maxLogEntries       = 1000
	defaultNetworkLimit = 30
	defaultConsoleLimit = 20
	maxURLLength        = 150
)

// truncateURL shortens a URL to maxURLLength characters.
func truncateURL(u string) string {
	if len(u) <= maxURLLength {
		return u
	}
	return u[:maxURLLength] + "…"
}

// lastN returns the last n elements of s. If n <= 0 or n >= len(s), returns s unchanged.
func lastN[T any](s []T, n int) []T {
	if n <= 0 || n >= len(s) {
		return s
	}
	return s[len(s)-n:]
}

// NetworkEntry is a captured network request/response.
type NetworkEntry struct {
	Method     string `json:"method"`
	URL        string `json:"url"`
	Status     int    `json:"status,omitempty"`
	StatusText string `json:"status_text,omitempty"`
	MimeType   string `json:"mime_type,omitempty"`
	BodySize   int    `json:"body_size,omitempty"`
	Error      string `json:"error,omitempty"`
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

// FilterNetwork returns entries matching the given URL substring.
func (c *LogCollector) FilterNetwork(urlSubstr string) []NetworkEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	var filtered []NetworkEntry
	for _, e := range c.network {
		if strings.Contains(e.URL, urlSubstr) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// Collect returns all accumulated entries.
func (c *LogCollector) Collect() ([]NetworkEntry, []ConsoleEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]NetworkEntry{}, c.network...), append([]ConsoleEntry{}, c.console...)
}

// SubscribeCDP enables the Network domain and starts collecting network events.
// Console events are also captured if Runtime is enabled elsewhere (e.g. via SubscribeConsole).
// Call this after page creation and before navigation.
// The event listener runs in a background goroutine until the page is closed.
func (c *LogCollector) SubscribeCDP(page *rod.Page) {
	_ = proto.NetworkEnable{}.Call(page)

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
					c.network[i].StatusText = e.Response.StatusText
					c.network[i].MimeType = e.Response.MIMEType
					c.network[i].BodySize = int(e.Response.EncodedDataLength)
					break
				}
			}
		},
		func(e *proto.NetworkLoadingFailed) {
			c.mu.Lock()
			defer c.mu.Unlock()
			for i := len(c.network) - 1; i >= 0; i-- {
				if c.network[i].Status == 0 {
					c.network[i].Error = e.ErrorText
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

// SubscribeConsole enables the Runtime domain for console log capture.
// This is a known CDP detection vector (Castle.io and similar services detect
// RuntimeEnable). Only call this when console logging is explicitly needed.
func (c *LogCollector) SubscribeConsole(page *rod.Page) {
	_ = proto.RuntimeEnable{}.Call(page)
}
