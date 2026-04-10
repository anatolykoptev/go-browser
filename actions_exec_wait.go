package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func doWaitFor(ctx context.Context, page *rod.Page, selector string, refMap *RefMap) error {
	if _, err := resolveElement(ctx, page, selector, refMap); err != nil {
		return fmt.Errorf("wait_for %q: %w", selector, err)
	}
	return nil
}

// doWaitForText polls until text appears in page body.
func doWaitForText(ctx context.Context, page *rod.Page, text string) error {
	for {
		content, err := proto.RuntimeEvaluate{
			Expression:    "document.body ? document.body.innerText : ''",
			ReturnByValue: true,
		}.Call(page)
		if err == nil {
			if strings.Contains(fmt.Sprintf("%v", content.Result.Value.Val()), text) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_for text %q: %w", text, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// doWaitForTextGone polls until text disappears from page body.
func doWaitForTextGone(ctx context.Context, page *rod.Page, text string) error {
	for {
		content, err := proto.RuntimeEvaluate{
			Expression:    "document.body ? document.body.innerText : ''",
			ReturnByValue: true,
		}.Call(page)
		if err == nil {
			if !strings.Contains(fmt.Sprintf("%v", content.Result.Value.Val()), text) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_for text_gone %q: %w", text, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// doWaitForCookie polls until a cookie with the given name appears.
// Used for PerimeterX (_px3), DataDome (datadome), CF (__cf_bm) challenge cookies.
func doWaitForCookie(ctx context.Context, page *rod.Page, name string) error {
	for {
		cookies, err := page.Cookies(nil)
		if err == nil {
			for _, c := range cookies {
				if c.Name == name {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_for cookie %q: %w", name, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func doSleep(ctx context.Context, waitMs int) error {
	if waitMs <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
		return nil
	}
}

// doWaitForNavigation polls until the page URL changes from startURL.
// If urlContains is non-empty, the new URL must also contain that substring.
// If selector is non-empty, waits for the element after URL change.
// Returns map with "url" and "title" (and "timeout"="true" on timeout).
func doWaitForNavigation(ctx context.Context, page *rod.Page, urlContains, selector string, refMap *RefMap) (map[string]string, error) {
	startURL := page.MustInfo().URL
	for {
		select {
		case <-ctx.Done():
			info, _ := page.Info()
			cur := startURL
			title := ""
			if info != nil {
				cur = info.URL
				title = info.Title
			}
			return map[string]string{"url": cur, "title": title, "timeout": "true"},
				fmt.Errorf("wait_for_navigation: %w", ctx.Err())
		case <-time.After(250 * time.Millisecond):
			info, err := page.Info()
			if err != nil {
				continue
			}
			currentURL := info.URL
			if currentURL == startURL {
				continue
			}
			if urlContains != "" && !strings.Contains(currentURL, urlContains) {
				continue
			}
			if selector != "" {
				if _, err := resolveElement(ctx, page, selector, refMap); err != nil {
					return map[string]string{"url": currentURL, "title": info.Title, "timeout": "true"},
						fmt.Errorf("wait_for_navigation selector %q: %w", selector, err)
				}
			}
			return map[string]string{"url": currentURL, "title": info.Title}, nil
		}
	}
}

func doWaitForStealth(ctx context.Context, page *rod.Page, selector string, refMap *RefMap) error {
	for {
		_, err := resolveRefNodeID(page, selector, refMap)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait_for %q: %w", selector, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}
