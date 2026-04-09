package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anatolykoptev/go-browser/cdputil"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func doWaitFor(ctx context.Context, page *rod.Page, selector string) error {
	if _, err := resolveElement(ctx, page, selector); err != nil {
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

func doWaitForStealth(ctx context.Context, page *rod.Page, selector string) error {
	for {
		_, err := cdputil.QuerySelector(page, selector)
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
