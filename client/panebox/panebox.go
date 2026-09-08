// Package panebox holds the geometry for a pane's interior boxes: the content
// area, the text-overlay textarea, and the hit-tests inside the pane border.
// It is outside client/wasm so go test exercises the math without a browser.
package panebox

import (
	"github.com/josephburnett/gridwell/client/pane"
	"github.com/josephburnett/gridwell/client/zoomtrans"
)

// LiveViewInsetPx is the one owner of the grab-gutter value. A
// WebContentsView eats all mouse input over its bounds, so the gap between two
// adjacent live panes, twice this inset, is the only canvas strip a user can
// click to grab a divider. At 5px per side that gap is about 10px, close to
// pane's resizeBandPx.
const LiveViewInsetPx = 5.0

// ContentBox returns the pane shrunk by borderPx on every side. URL tiles
// render into it and the URL stream mouse handlers hit-test against it.
func ContentBox(r pane.Rect, borderPx float64) pane.Rect {
	x := r.X + borderPx
	y := r.Y + borderPx
	w := r.W - 2*borderPx
	h := r.H - 2*borderPx
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return pane.Rect{X: x, Y: y, W: w, H: h}
}

// PointInContent reports whether (sx, sy) lies inside ContentBox(r, borderPx).
// Every live surface fills that box, as does the canvas frame drawn in its
// place while it is parked.
func PointInContent(r pane.Rect, borderPx, sx, sy float64) bool {
	return ContentBox(r, borderPx).Contains(sx, sy)
}

// LiveViewOwnsPoint decides whether a pane's live view owns a screen point.
// Every canvas pointer handler asks it before handing an event to the native
// surface. A WebContentsView paints over the content box and swallows the
// mouse there, unless overlaysHidden (the shim's liveOverlaysHidden parks every
// view during a gesture, so the canvas keeps the release that ends it) or the
// pane has no live view, a frozen preview being only a canvas drawing.
func LiveViewOwnsPoint(overlaysHidden, hasLiveView bool, r pane.Rect, borderPx, x, y float64) bool {
	if overlaysHidden || !hasLiveView {
		return false
	}
	return PointInContent(r, borderPx, x, y)
}

// TextareaBox returns the text-overlay textarea rectangle and its rendered
// font size. sideInset is the gap between the pane edge and the text.
func TextareaBox(r pane.Rect, sideInset, baseFontPx, scale float64) (rect pane.Rect, fontPx float64) {
	fontPx = baseFontPx * scale
	x := r.X + sideInset
	y := r.Y + sideInset
	w := r.W - 2*sideInset
	h := r.H - 2*sideInset
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return pane.Rect{X: x, Y: y, W: w, H: h}, fontPx
}

// InnerBox is the text-focused pane's inner reading area, the textarea's
// rectangle without the font size.
func InnerBox(r pane.Rect, sideInset float64) pane.Rect {
	b, _ := TextareaBox(r, sideInset, 0, 0)
	return b
}

// PointInInner reports whether (sx, sy) lies inside InnerBox(r, sideInset).
func PointInInner(r pane.Rect, sideInset, sx, sy float64) bool {
	return InnerBox(r, sideInset).Contains(sx, sy)
}

// FitZoom is zoomtrans.Fit against the pane's inner box. A degenerate inner
// box returns 1.
func FitZoom(r pane.Rect, fileW, fileH int64, sideInset, cellPx float64) float64 {
	inner := InnerBox(r, sideInset)
	if inner.W <= 0 || inner.H <= 0 {
		return 1
	}
	return zoomtrans.Fit(fileW, fileH, inner.W, inner.H, cellPx)
}

// ModalCardPos centers a modal card on the active pane rather than the screen,
// clamped so a small pane near an edge cannot push it off the window. A card
// larger than the window on an axis pins to 0, keeping its first input
// reachable.
func ModalCardPos(paneRect pane.Rect, cardW, cardH, winW, winH float64) (x, y float64) {
	x = paneRect.X + paneRect.W/2 - cardW/2
	y = paneRect.Y + paneRect.H/2 - cardH/2
	x = clampAxis(x, cardW, winW)
	y = clampAxis(y, cardH, winH)
	return x, y
}

// clampAxis keeps [pos, pos+size] inside [0, limit], preferring 0 when size
// exceeds limit.
func clampAxis(pos, size, limit float64) float64 {
	if pos+size > limit {
		pos = limit - size
	}
	if pos < 0 {
		pos = 0
	}
	return pos
}
