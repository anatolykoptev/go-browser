package browser

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

func init() {
	registerAction("wait_stable", execWaitStable)
}

func execWaitStable(dc dispatchContext, a Action) (any, error) {
	return nil, doWaitStable(&dc, a)
}

const (
	defaultWaitStableQuiet   = 500
	defaultWaitStableMaxWait = 10_000
)

// doWaitStable returns when the page has had `quiet_ms` of no network requests
// (excluding ignored hosts) AND no DOM mutations. Fails if `max_wait_ms` elapses.
func doWaitStable(dc *dispatchContext, a Action) error {
	quiet := a.QuietMs
	if quiet <= 0 {
		quiet = defaultWaitStableQuiet
	}
	maxWait := a.MaxWaitMs
	if maxWait <= 0 {
		maxWait = defaultWaitStableMaxWait
	}
	ignore := make(map[string]bool, len(a.IgnoreHosts))
	for _, h := range a.IgnoreHosts {
		ignore[strings.ToLower(h)] = true
	}

	var (
		mu           sync.Mutex
		lastActivity = time.Now()
	)
	bump := func() {
		mu.Lock()
		lastActivity = time.Now()
		mu.Unlock()
	}

	// Subscribe to network + DOM events.
	ctx, cancel := context.WithTimeout(dc.ctx, time.Duration(maxWait)*time.Millisecond)
	defer cancel()

	// Run EachEvent in a goroutine: the returned wait func blocks until all
	// handlers return true — void handlers never do, so calling it directly
	// (or via defer) would hang forever. Goroutine is abandoned; handlers
	// clean up when the page is closed. See logs.go:179 for the same idiom.
	go dc.page.EachEvent(
		func(e *proto.NetworkRequestWillBeSent) {
			if hostIgnored(e.Request.URL, ignore) {
				return
			}
			bump()
		},
		func(e *proto.NetworkLoadingFinished) { bump() },
		func(e *proto.NetworkLoadingFailed) { bump() },
		func(e *proto.DOMDocumentUpdated) { bump() },
	)()

	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_stable: page never settled within %dms", maxWait)
		case <-tick.C:
			mu.Lock()
			idle := time.Since(lastActivity)
			mu.Unlock()
			if idle >= time.Duration(quiet)*time.Millisecond {
				return nil
			}
		}
	}
}

func hostIgnored(rawURL string, ignore map[string]bool) bool {
	if len(ignore) == 0 {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Host)
	for ig := range ignore {
		if h == ig || strings.HasSuffix(h, "."+ig) {
			return true
		}
	}
	return false
}
