package browser

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const (
	maxLogEntries       = 1000
	defaultNetworkLimit = 30
	defaultConsoleLimit = 20
	maxURLLength        = 150
)

// truncateURL shortens a URL to maxURLLength runes.
// Uses rune-aware slicing to avoid splitting multi-byte UTF-8 sequences.
func truncateURL(u string) string {
	if utf8.RuneCountInString(u) <= maxURLLength {
		return u
	}
	// Walk runes until we reach the limit, tracking the byte index.
	byteIdx, count := 0, 0
	for byteIdx < len(u) && count < maxURLLength {
		_, size := utf8.DecodeRuneInString(u[byteIdx:])
		byteIdx += size
		count++
	}
	return u[:byteIdx] + "…"
}

// lastN returns the last n elements of s.
// If n <= 0, returns an empty slice.
// If n >= len(s), returns all elements unchanged.
func lastN[T any](s []T, n int) []T {
	if n <= 0 {
		return s[:0]
	}
	if n >= len(s) {
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
	TS         int64  `json:"ts"` // Unix ms timestamp when entry was recorded
}

// ConsoleEntry is a captured console API call.
type ConsoleEntry struct {
	Level string `json:"level"`
	Text  string `json:"text"`
	TS    int64  `json:"ts"` // Unix ms timestamp when entry was recorded
}

// ExceptionEntry is a captured JavaScript exception or promise rejection.
type ExceptionEntry struct {
	TS           int64  `json:"ts"` // Unix ms timestamp when entry was recorded
	Text         string `json:"text"`
	URL          string `json:"url,omitempty"`
	LineNumber   int    `json:"line_number,omitempty"`
	ColumnNumber int    `json:"column_number,omitempty"`
	StackTrace   string `json:"stack_trace,omitempty"`
}

// NavigationEntry is a captured main-frame navigation event.
type NavigationEntry struct {
	URL string `json:"url"`
	TS  int64  `json:"ts"` // Unix ms timestamp when entry was recorded
}

// LogCollector accumulates network, console, exception, and navigation entries from CDP events.
type LogCollector struct {
	mu          sync.Mutex
	network     []NetworkEntry
	console     []ConsoleEntry
	exceptions  []ExceptionEntry
	navigations []NavigationEntry
}

// NewLogCollector creates an empty log collector.
func NewLogCollector() *LogCollector {
	return &LogCollector{
		network:     make([]NetworkEntry, 0, 64),
		console:     make([]ConsoleEntry, 0, 64),
		exceptions:  make([]ExceptionEntry, 0, 16),
		navigations: make([]NavigationEntry, 0, 16),
	}
}

// AddNetwork appends a network entry with ring buffer semantics (drops oldest when full).
func (c *LogCollector) AddNetwork(e NetworkEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.network) < maxLogEntries {
		c.network = append(c.network, e)
	} else {
		// Drop oldest entry and append new one
		copy(c.network[0:], c.network[1:])
		c.network[len(c.network)-1] = e
	}
}

// AddConsole appends a console entry with ring buffer semantics (drops oldest when full).
func (c *LogCollector) AddConsole(e ConsoleEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.console) < maxLogEntries {
		c.console = append(c.console, e)
	} else {
		// Drop oldest entry and append new one
		copy(c.console[0:], c.console[1:])
		c.console[len(c.console)-1] = e
	}
}

// AddException appends an exception entry with ring buffer semantics (drops oldest when full).
func (c *LogCollector) AddException(e ExceptionEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.exceptions) < maxLogEntries {
		c.exceptions = append(c.exceptions, e)
	} else {
		// Drop oldest entry and append new one
		copy(c.exceptions[0:], c.exceptions[1:])
		c.exceptions[len(c.exceptions)-1] = e
	}
}

// AddNavigation appends a navigation entry with ring buffer semantics (drops oldest when full).
func (c *LogCollector) AddNavigation(e NavigationEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.navigations) < maxLogEntries {
		c.navigations = append(c.navigations, e)
	} else {
		// Drop oldest entry and append new one
		copy(c.navigations[0:], c.navigations[1:])
		c.navigations[len(c.navigations)-1] = e
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

// SubscribeCDP enables the Network, Runtime, and Page domains and starts collecting all events.
// Call this after page creation and before navigation.
// The event listener runs in a background goroutine until the page is closed.
func (c *LogCollector) SubscribeCDP(page *rod.Page) {
	_ = proto.NetworkEnable{}.Call(page)
	_ = proto.RuntimeEnable{}.Call(page)
	_ = proto.PageEnable{}.Call(page)

	go page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			// Add as network entry
			c.AddNetwork(NetworkEntry{
				Method: e.Request.Method,
				URL:    e.Request.URL,
				TS:     time.Now().UnixMilli(),
			})
		},
		func(e *proto.PageFrameNavigated) {
			// Main frame has no parent. Sub-frames/iframes have ParentID set.
			if e.Frame != nil && e.Frame.ParentID == "" {
				c.AddNavigation(NavigationEntry{
					TS:  time.Now().UnixMilli(),
					URL: e.Frame.URL,
				})
			}
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
					c.network[i].TS = time.Now().UnixMilli()
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
					c.network[i].TS = time.Now().UnixMilli()
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
				TS:    time.Now().UnixMilli(),
			})
		},
		func(e *proto.RuntimeExceptionThrown) {
			if e.ExceptionDetails == nil {
				return
			}
			ed := e.ExceptionDetails
			// Build best-available description. ed.Text (e.g. "Uncaught") is
			// always set. Exception.Description has the full stack for Error
			// objects; Exception.Value is set when a primitive is thrown.
			text := ed.Text
			if ed.Exception != nil {
				if desc := ed.Exception.Description; desc != "" {
					text = desc
				} else if s := ed.Exception.Value.Str(); s != "" {
					text = ed.Text + ": " + s
				}
			}
			if text == "" {
				text = "unknown exception"
			}
			entry := ExceptionEntry{
				TS:           time.Now().UnixMilli(),
				Text:         text,
				URL:          ed.URL,
				LineNumber:   int(ed.LineNumber),
				ColumnNumber: int(ed.ColumnNumber),
			}
			if ed.StackTrace != nil {
				var b strings.Builder
				for _, f := range ed.StackTrace.CallFrames {
					fmt.Fprintf(&b, "  at %s (%s:%d:%d)\n", f.FunctionName, f.URL, f.LineNumber, f.ColumnNumber)
				}
				entry.StackTrace = b.String()
			}
			c.AddException(entry)
		},
	)()
}

// SubscribeConsole enables the Runtime domain for console log capture.
// This is a known CDP detection vector (Castle.io and similar services detect
// RuntimeEnable). Only call this when console logging is explicitly needed.
func (c *LogCollector) SubscribeConsole(page *rod.Page) {
	_ = proto.RuntimeEnable{}.Call(page)
}

// SinceFilter restricts what Since returns. Zero-value = no filtering.
type SinceFilter struct {
	Kinds     []string // "network" | "console" | "exceptions" | "navigations" — if empty, all
	StatusMin int      // network only: min HTTP status (e.g. 400 for errors only)
	Limit     int      // max entries per category (0 = no limit)
}

// SinceResult contains filtered log entries since a given timestamp.
type SinceResult struct {
	Network     []NetworkEntry    `json:"network"`
	Console     []ConsoleEntry    `json:"console"`
	Exceptions  []ExceptionEntry  `json:"exceptions"`
	Navigations []NavigationEntry `json:"navigations"`
}

// Since returns all entries with timestamp > sinceMs (exclusive).
func (c *LogCollector) Since(sinceMs int64) SinceResult {
	return c.SinceFiltered(sinceMs, SinceFilter{})
}

// SinceFiltered is Since with additional server-side filtering.
// Reduces payload size when agents only want errors or a narrow slice.
func (c *LogCollector) SinceFiltered(sinceMs int64, f SinceFilter) SinceResult {
	kinds := map[string]bool{}
	for _, k := range f.Kinds {
		kinds[k] = true
	}
	include := func(k string) bool { return len(kinds) == 0 || kinds[k] }

	c.mu.Lock()
	defer c.mu.Unlock()
	out := SinceResult{}
	if include("network") {
		for _, e := range c.network {
			if e.TS <= sinceMs {
				continue
			}
			if f.StatusMin > 0 && e.Status < f.StatusMin {
				continue
			}
			out.Network = append(out.Network, e)
			if f.Limit > 0 && len(out.Network) >= f.Limit {
				break
			}
		}
	}
	if include("console") {
		for _, e := range c.console {
			if e.TS <= sinceMs {
				continue
			}
			out.Console = append(out.Console, e)
			if f.Limit > 0 && len(out.Console) >= f.Limit {
				break
			}
		}
	}
	if include("exceptions") {
		for _, e := range c.exceptions {
			if e.TS <= sinceMs {
				continue
			}
			out.Exceptions = append(out.Exceptions, e)
			if f.Limit > 0 && len(out.Exceptions) >= f.Limit {
				break
			}
		}
	}
	if include("navigations") {
		for _, e := range c.navigations {
			if e.TS <= sinceMs {
				continue
			}
			out.Navigations = append(out.Navigations, e)
			if f.Limit > 0 && len(out.Navigations) >= f.Limit {
				break
			}
		}
	}
	return out
}
