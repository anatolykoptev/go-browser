package browser

import (
	"context"
	"fmt"
	"os"
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
	subMu       sync.Mutex
	subCancel   context.CancelFunc
	subCtx      context.Context
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

// Stats returns the current count of captured entries per category. Non-blocking,
// lock held briefly. Intended for periodic Prometheus scraping. Note: because the
// underlying buffers are ring buffers, these counts can decrease when the oldest
// entry is evicted after reaching maxLogEntries.
func (c *LogCollector) Stats() (network, console, exceptions, navigations int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.network), len(c.console), len(c.exceptions), len(c.navigations)
}

// Collect returns all accumulated entries.
func (c *LogCollector) Collect() ([]NetworkEntry, []ConsoleEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]NetworkEntry{}, c.network...), append([]ConsoleEntry{}, c.console...)
}

// SubscribeCDP enables the Network, Runtime, and Page domains and starts collecting all events.
// Call this after page creation and before navigation.
// The event listener runs in a background goroutine until the page is closed or Resubscribe is called.
func (c *LogCollector) SubscribeCDP(page *rod.Page) {
	c.startSubscription(page)
}

// Resubscribe cancels any existing EachEvent goroutine and starts a fresh one on the given page.
// Use this after navigation: Chrome emits Runtime.executionContextsCleared which invalidates
// cached Runtime-domain state inside rod; the old goroutine keeps running but receives no
// consoleAPICalled/exceptionThrown events. Network/Page events continue unaffected because they
// are emitted at the page/target layer.
func (c *LogCollector) Resubscribe(page *rod.Page) {
	c.startSubscription(page)
}

func (c *LogCollector) startSubscription(page *rod.Page) {
	// Serialize subscription swaps so two concurrent callers don't race the
	// cancel/start sequence.
	c.subMu.Lock()
	defer c.subMu.Unlock()
	if c.subCancel != nil {
		c.subCancel()
	}

	// Enable domains directly (bypass rod's state cache — it can be stale
	// after a prior subscription was cancelled, which restores "disabled"
	// lazily inside the defunct goroutine).
	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		fmt.Fprintf(os.Stderr, "logs.SubscribeCDP: Network.enable err=%v\n", err)
	}
	if err := (proto.RuntimeEnable{}).Call(page); err != nil {
		fmt.Fprintf(os.Stderr, "logs.SubscribeCDP: Runtime.enable err=%v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "logs.SubscribeCDP: Runtime.enable OK sessionID=%s\n", page.SessionID)
	}
	if err := (proto.PageEnable{}).Call(page); err != nil {
		fmt.Fprintf(os.Stderr, "logs.SubscribeCDP: Page.enable err=%v\n", err)
	}

	ctx, cancel := context.WithCancel(page.GetContext())
	c.subCtx = ctx
	c.subCancel = cancel

	// Subscribe directly to the page's CDP message stream instead of rod's
	// EachEvent: EachEvent owns the per-session EnableDomain state machine,
	// and when its wait-goroutine exits it "restores" (i.e. disables) the
	// domains it enabled. That caused Runtime events to stop arriving on the
	// next call. Manual Load/dispatch doesn't touch that cache at all.
	msgs := page.Context(ctx).Event()
	go c.runLoop(ctx, msgs)
}

func (c *LogCollector) runLoop(ctx context.Context, msgs <-chan *rod.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			c.dispatch(msg)
		}
	}
}

func (c *LogCollector) dispatch(msg *rod.Message) {
	if strings.HasPrefix(msg.Method, "Runtime.") || strings.HasPrefix(msg.Method, "Log.") {
		fmt.Fprintf(os.Stderr, "CDP-DISPATCH method=%s sessionID=%s\n", msg.Method, msg.SessionID)
	}
	switch msg.Method {
	case (&proto.NetworkRequestWillBeSent{}).ProtoEvent():
		var e proto.NetworkRequestWillBeSent
		if msg.Load(&e) {
			c.AddNetwork(NetworkEntry{
				Method: e.Request.Method,
				URL:    e.Request.URL,
				TS:     time.Now().UnixMilli(),
			})
		}
	case (&proto.PageFrameNavigated{}).ProtoEvent():
		var e proto.PageFrameNavigated
		if msg.Load(&e) && e.Frame != nil && e.Frame.ParentID == "" {
			c.AddNavigation(NavigationEntry{TS: time.Now().UnixMilli(), URL: e.Frame.URL})
		}
	case (&proto.NetworkResponseReceived{}).ProtoEvent():
		var e proto.NetworkResponseReceived
		if msg.Load(&e) {
			c.mu.Lock()
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
			c.mu.Unlock()
		}
	case (&proto.NetworkLoadingFailed{}).ProtoEvent():
		var e proto.NetworkLoadingFailed
		if msg.Load(&e) {
			c.mu.Lock()
			for i := len(c.network) - 1; i >= 0; i-- {
				if c.network[i].Status == 0 {
					c.network[i].Error = e.ErrorText
					c.network[i].TS = time.Now().UnixMilli()
					break
				}
			}
			c.mu.Unlock()
		}
	case (&proto.RuntimeConsoleAPICalled{}).ProtoEvent():
		var e proto.RuntimeConsoleAPICalled
		if msg.Load(&e) {
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
		}
	case (&proto.RuntimeExceptionThrown{}).ProtoEvent():
		var e proto.RuntimeExceptionThrown
		if msg.Load(&e) && e.ExceptionDetails != nil {
			ed := e.ExceptionDetails
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
		}
	}
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
