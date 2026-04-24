package browser

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

// TestDoEvaluate_BasicShapes smoke-tests the doEvaluate wrapper against a live
// Chromium page. The wrapper stringifies values in the renderer to avoid CDP's
// -32000 "Object reference chain is too long" on DOM-adjacent graphs. These
// cases were previously surfaced as ErrJsException by the naive ReturnByValue
// path.
func TestDoEvaluate_BasicShapes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}
	b := acquireSharedBrowser(t)

	const html = `data:text/html,<div class="cell">hi</div><button class="info-btn" style="position:absolute;left:10px;top:20px;width:100px;height:30px">btn</button>`
	page, err := b.Page(proto.TargetCreateTarget{URL: html})
	if err != nil {
		t.Fatalf("open page: %v", err)
	}
	defer func() { _ = page.Close() }()

	// 1. Bare expression returning a plain value.
	v, err := doEvaluate(page, `document.title`)
	if err != nil {
		t.Fatalf("bare expression: %v", err)
	}
	if _, ok := v.(string); !ok {
		t.Fatalf("bare expression: want string, got %T (%v)", v, v)
	}

	// 2. IIFE returning a plain object — the regression case. Previously this
	// triggered CDP's "Object reference chain is too long" on some DOM graphs
	// and got mis-surfaced as js_exception.
	v, err = doEvaluate(page, `(() => { const r = document.querySelector('.info-btn').getBoundingClientRect(); return { top: r.top, h: r.height }; })()`)
	if err != nil {
		t.Fatalf("IIFE bounding rect: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("IIFE bounding rect: want map, got %T (%v)", v, v)
	}
	if _, has := m["top"]; !has {
		t.Fatalf("IIFE bounding rect: missing 'top' key: %v", m)
	}

	// 3. DOM node reference — JSON.stringify yields null for a raw Element,
	// so the caller sees nil. Previously this path could trip -32000.
	v, err = doEvaluate(page, `document.querySelector('.cell')`)
	if err != nil {
		t.Fatalf("DOM node ref: %v", err)
	}
	if v != nil {
		t.Logf("DOM node ref returned %T (%v) — browser may have serialized it", v, v)
	}

	// 4. User script that throws — must surface as ErrJsException with the
	// real message.
	_, err = doEvaluate(page, `(() => { throw new Error("boom"); })()`)
	if err == nil {
		t.Fatal("user throw: want error, got nil")
	}
	if !errors.Is(err, ErrJsException) {
		t.Errorf("user throw: want ErrJsException, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("user throw: error should preserve message, got %v", err)
	}

	// 5. Number / boolean primitives.
	v, err = doEvaluate(page, `1+1`)
	if err != nil {
		t.Fatalf("arithmetic: %v", err)
	}
	if f, ok := v.(float64); !ok || f != 2 {
		t.Errorf("arithmetic: want 2, got %T (%v)", v, v)
	}

	v, err = doEvaluate(page, `true`)
	if err != nil {
		t.Fatalf("boolean: %v", err)
	}
	if b, ok := v.(bool); !ok || !b {
		t.Errorf("boolean: want true, got %T (%v)", v, v)
	}
}
