package preview

// Fit geometry for URL and shell tile previews. An image scales uniformly to
// fit inside the destination rect, centered, with letterbox or pillarbox bars
// filling the remainder, the way CSS object-fit: contain does. A preview whose
// aspect differs sharply from the pane reads better letterboxed than
// cover-cropped, because the whole capture stays visible. Pure floats, so it is
// testable without a canvas.

// ContainDstRect returns the sub-rectangle inside (x, y, w, h) that an iw by ih
// image fills when scaled by the smaller axis ratio and centered, so bars
// appear on at most one axis. ok is false for a degenerate image or
// destination, and the caller then stretch-draws the whole image.
func ContainDstRect(iw, ih, x, y, w, h float64) (dx, dy, dw, dh float64, ok bool) {
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return x, y, w, h, false
	}
	scale := w / iw
	if s := h / ih; s < scale {
		scale = s
	}
	dw = iw * scale
	dh = ih * scale
	return x + (w-dw)/2, y + (h-dh)/2, dw, dh, true
}

// StandinDstRect places a live-surface snapshot back where the live surface
// drew it: anchored at the top left of the content box at its intrinsic CSS
// size, natural pixels over the capture-time device pixel ratio, never scaled.
// A live xterm canvas is a whole number of cells and so a little smaller than
// its content box, and contain-fitting its snapshot would center and enlarge it
// by the leftover cell fraction, shifting the terminal pixels every time the
// overlay parks. ok is false for a degenerate image, and a non-positive dpr
// counts as 1.
func StandinDstRect(iw, ih, dpr, x, y float64) (dx, dy, dw, dh float64, ok bool) {
	if iw <= 0 || ih <= 0 {
		return 0, 0, 0, 0, false
	}
	if dpr <= 0 {
		dpr = 1
	}
	return x, y, iw / dpr, ih / dpr, true
}
