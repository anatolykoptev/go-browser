package humanize

import (
	"encoding/json"

	"github.com/go-rod/rod"
)

const (
	viewportFallbackW = 1440.0
	viewportFallbackH = 900.0
	viewportMargin    = 50.0
)

// ViewportBounds holds the usable mouse area within the viewport.
type ViewportBounds struct {
	MinX, MaxX float64
	MinY, MaxY float64
}

// ReadViewport reads the actual inner dimensions of the page window.
// On any error it returns 1440×900 as a safe fallback.
func ReadViewport(page *rod.Page) (width, height float64, err error) {
	val, err := page.Eval(`() => JSON.stringify({w: window.innerWidth, h: window.innerHeight})`)
	if err != nil {
		return viewportFallbackW, viewportFallbackH, nil
	}

	var vp struct {
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	if err := json.Unmarshal([]byte(val.Value.String()), &vp); err != nil {
		return viewportFallbackW, viewportFallbackH, nil
	}
	if vp.W <= 0 || vp.H <= 0 {
		return viewportFallbackW, viewportFallbackH, nil
	}
	return vp.W, vp.H, nil
}

// ViewportInnerBounds returns the safe mouse area with a margin applied.
// Margin defaults to viewportMargin if not provided.
func ViewportInnerBounds(page *rod.Page) ViewportBounds {
	w, h, _ := ReadViewport(page)
	return ViewportBounds{
		MinX: viewportMargin,
		MaxX: w - viewportMargin,
		MinY: viewportMargin,
		MaxY: h - viewportMargin,
	}
}
