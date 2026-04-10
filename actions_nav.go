package browser

import (
	"github.com/anatolykoptev/go-browser/cdputil"
)

// actions_nav.go — executors for navigate, scroll, resize actions.

func init() {
	registerAction("navigate", execNavigate)
	registerAction("scroll", execScroll)
	registerAction("resize", execResize)
}

func execNavigate(dc dispatchContext, a Action) (any, error) {
	return nil, doNavigate(dc.ctx, dc.page, a.URL)
}

func execScroll(dc dispatchContext, a Action) (any, error) {
	if dc.stealthMode && a.Selector != "" {
		nodeID, err := resolveRefNodeID(dc.page, a.Selector, dc.refMap)
		if err != nil {
			return nil, err
		}
		return nil, cdputil.ScrollIntoView(dc.page, nodeID)
	}
	if dc.stealthMode && a.Selector == "" && a.DeltaY != 0 {
		return nil, doScrollHumanized(dc.ctx, dc.page, int(a.DeltaY), dc.cursor)
	}
	return nil, doScroll(dc.ctx, dc.page, a.Selector, a.DeltaX, a.DeltaY, dc.refMap)
}

func execResize(dc dispatchContext, a Action) (any, error) {
	return nil, doResize(dc.page, a.Width, a.Height)
}
