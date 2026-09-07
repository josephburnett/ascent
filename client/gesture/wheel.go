package gesture

// WheelAction is what a wheel event over a pane does.
type WheelAction int

const (
	// WheelZoomPane is the pane-wide cursor-anchored zoom
	// (zoomtrans.WheelZoom).
	WheelZoomPane WheelAction = iota
	// WheelZoomWell zooms the grid inside the hovered well, through its
	// stored preview framing, rather than the grid the pane shows.
	WheelZoomWell
	// WheelScrollDoc scrolls a rendered-mode text descent vertically.
	WheelScrollDoc
	// WheelSwallow drops a stray wheel that reached the canvas under a live
	// url view, so it cannot zoom the pane underneath.
	WheelSwallow
	// WheelIgnore leaves the canvas alone, as when a textarea overlay
	// handles its own scrolling.
	WheelIgnore
)

// WheelInput is the state ClassifyWheel decides on. The caller resolves the
// impure facts.
type WheelInput struct {
	// TextFocused means the pane is descended into a content tile.
	TextFocused bool
	// URLDescent means that descent is into a url tile.
	URLDescent bool
	// LiveURLView means a native WebContentsView is attached for this pane.
	LiveURLView bool
	// InContentBox means the cursor is inside the pane's content box.
	InContentBox bool
	// TextModeRendered means the text descent is in rendered mode.
	TextModeRendered bool
	// OverEnterableWell means the cursor is over a well tile with a
	// resolvable child grid, the same predicate a drop's PromoteToWell
	// uses.
	OverEnterableWell bool
	// ZoomOut means the wheel direction shrinks, deltaY > 0 in
	// zoomtrans.WheelZoom's convention.
	ZoomOut bool
	// WellCoverage is how much of the pane's content box the hovered well
	// covers, 0 to 1 (RectCoverage). It means nothing without
	// OverEnterableWell.
	WellCoverage float64
}

// WellZoomOutRedirect is the coverage past which zooming out over a well
// zooms the pane instead. A well that fills most of the view leaves no
// visible outer context, so wheeling out inside it asks to back out. Zooming
// in stays with the well at any coverage.
const WellZoomOutRedirect = 0.5

// RectCoverage is the share of the box (bx,by,bw,bh) the rect (rx,ry,rw,rh)
// covers, from 0 to 1. It is the geometry behind WellZoomOutRedirect.
func RectCoverage(rx, ry, rw, rh, bx, by, bw, bh float64) float64 {
	if bw <= 0 || bh <= 0 {
		return 0
	}
	ix := max(rx, bx)
	iy := max(ry, by)
	ix2 := min(rx+rw, bx+bw)
	iy2 := min(ry+rh, by+bh)
	if ix2 <= ix || iy2 <= iy {
		return 0
	}
	return ((ix2 - ix) * (iy2 - iy)) / (bw * bh)
}

// ClassifyWheel routes a wheel event. Outside a content descent the wheel
// zooms the pane, except over an enterable well, where it zooms that well's
// own preview. Inside a descent a live url view over the content box swallows
// strays because the view scrolls itself, and rendered mode scrolls the
// document.
func ClassifyWheel(in WheelInput) WheelAction {
	if !in.TextFocused {
		if in.OverEnterableWell {
			if in.ZoomOut && in.WellCoverage > WellZoomOutRedirect {
				return WheelZoomPane
			}
			return WheelZoomWell
		}
		return WheelZoomPane
	}
	if in.URLDescent && in.LiveURLView && in.InContentBox {
		return WheelSwallow
	}
	if in.TextModeRendered {
		return WheelScrollDoc
	}
	return WheelIgnore
}
