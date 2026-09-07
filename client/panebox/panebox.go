// Package panebox holds the geometry for a pane's interior boxes: the content
// area, the text-overlay textarea, and the hit-tests inside the pane border.
// It lives outside client/wasm so go test exercises the math without a browser.
// Every function takes a pane.Rect, the same screen-space rectangle the layout
// and dragdrop code uses.
package panebox

import (
	"github.com/josephburnett/gridwell/client/pane"
	"github.com/josephburnett/gridwell/client/zoomtrans"
)

// LiveViewInsetPx is the inset on every side of a pane's live content view,
// meaning a URL WebContentsView or a shell overlay, and it is the one owner of
// the grab-gutter value.
//
// A WebContentsView eats all mouse input over its bounds, so the gap between
// two adjacent live panes, twice this inset, is the only canvas strip a user
// can click to grab a divider. At 5px per side the gap is about 10px, close to
// pane's resizeBandPx and comfortably grabbable. client/wasm render.go and
// shell_stream_client.go read it through ContentBox rather than keeping copies.
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
// Every live surface fills that same box, as does the canvas frame drawn in its
// place while it is parked, so a parked frame lands where the live view was.
func PointInContent(r pane.Rect, borderPx, sx, sy float64) bool {
	return ContentBox(r, borderPx).Contains(sx, sy)
}

// LiveViewOwnsPoint owns whether a pane's live view owns a screen point. Every
// canvas pointer handler asks it before handing an event to the native surface
// instead of acting on the event itself.
//
// A url tile's WebContentsView paints over the pane's content box and swallows
// the mouse there, so a point inside that box belongs to the page. Two facts
// unmake that, and both are inputs here rather than assumptions at the call
// site:
//
//   - overlaysHidden. The client parks every live view while a gesture is
//     armed, the + menu is open, or the url modal is up; the shim's
//     liveOverlaysHidden owns that state. A parked view paints nothing and owns
//     nothing, so the canvas keeps every event over it, including the release
//     that ends the gesture that parked it. A handler that swallows without
//     asking discards that release and leaves the gesture armed forever.
//   - hasLiveView. A frozen preview is a canvas drawing, and only a live view
//     owns pixels.
func LiveViewOwnsPoint(overlaysHidden, hasLiveView bool, r pane.Rect, borderPx, x, y float64) bool {
	if overlaysHidden || !hasLiveView {
		return false
	}
	return PointInContent(r, borderPx, x, y)
}

// TextareaBox returns the text-overlay textarea rectangle and its rendered
// font size. sideInset is the gap between the pane edge and the text content,
// and scale multiplies baseFontPx.
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

// InnerBox is the text-focused pane's inner reading area, identical to the
// textarea's rectangle without the font size.
func InnerBox(r pane.Rect, sideInset float64) pane.Rect {
	b, _ := TextareaBox(r, sideInset, 0, 0)
	return b
}

// PointInInner reports whether (sx, sy) lies inside InnerBox(r, sideInset).
func PointInInner(r pane.Rect, sideInset, sx, sy float64) bool {
	return InnerBox(r, sideInset).Contains(sx, sy)
}

// FitZoom returns the zoom at which a text tile of fileW by fileH cells just
// fits the pane's inner box, which is zoomtrans.Fit. A degenerate inner box
// returns 1.
func FitZoom(r pane.Rect, fileW, fileH int64, sideInset, cellPx float64) float64 {
	inner := InnerBox(r, sideInset)
	if inner.W <= 0 || inner.H <= 0 {
		return 1
	}
	return zoomtrans.Fit(fileW, fileH, inner.W, inner.H, cellPx)
}

// ModalCardPos centers a modal card on the active pane rather than the screen,
// so the dialog appears in the pane you acted in. It returns the card's
// top-left, clamped so a small pane near an edge cannot push the card off the
// window. A card larger than the window on an axis pins to 0, keeping the
// top-left and its first input reachable.
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
