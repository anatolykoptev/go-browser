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
		nodeID, err := cdputil.QuerySelector(dc.page, a.Selector)
		if err != nil {
			return nil, err
		}
		return nil, cdputil.ScrollIntoView(dc.page, nodeID)
	}
	return nil, doScroll(dc.ctx, dc.page, a.Selector, a.DeltaX, a.DeltaY)
}

func execResize(dc dispatchContext, a Action) (any, error) {
	return nil, doResize(dc.page, a.Width, a.Height)
}
