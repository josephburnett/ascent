package markdown

// The scale, scroll and baseline math for a markdown tile's preview and its
// raw-text mode. It lives here rather than in the canvas painter
// (client/wasm/markdown_render.go) so `go test` executes it, and so the
// preview, the descended pane and the editing <textarea> place lines
// identically.

// PreviewFrame is the scale and scroll offset a markdown tile preview renders
// with.
type PreviewFrame struct {
	Scale            float64
	ScrollX, ScrollY float64
	// ContentW is the width the markdown wraps to: the tile's inner width
	// divided by Scale, so the painter only scales the ops back up.
	ContentW float64
}

// PreviewBodyLinePx is one body-text line's height at scale 1: 14px body
// times 1.35 line spacing, rounded up.
const PreviewBodyLinePx = 19.0

// PreviewWindowFrame scales a text tile preview. The type size is constant, so
// the font never follows grid zoom and the tile is a window that reveals more
// of the document as it grows. content_zoom is the one owner of making the
// text bigger.
func PreviewWindowFrame(innerW, fixedScale, contentZoom float64, storedX, storedY int64) PreviewFrame {
	s := fixedScale * contentZoom
	if s <= 0 {
		s = fixedScale
	}
	return PreviewFrame{
		Scale:    s,
		ScrollX:  float64(storedX),
		ScrollY:  float64(storedY),
		ContentW: innerW / s,
	}
}

// PreviewContentVisible gates the content paint: below one body line of room
// the preview is the alt-text banner alone, mirroring the well's previewCell
// >= 0.5 gate. availH is the tile's inner height minus the banner.
func PreviewContentVisible(availH, scale float64) bool {
	return availH >= PreviewBodyLinePx*scale
}

// RawTextSlot holds per-line placement for monospace raw text, matched to the
// editing <textarea>'s CSS line boxes to the pixel.
type RawTextSlot struct {
	Slot     float64 // line advance, in scaled px
	Baseline float64 // alphabetic-baseline offset from a slot's top
	Top0     float64 // top of the first line's slot, in scaled px
}

// RawTextLineSlot computes line slot geometry for raw monospace text, matching
// CSS: a line box is fontPx × lineHeightMul tall, the font's content area
// (asc+desc) is centered in it, and the alphabetic baseline sits one ascent
// below that area's top.
func RawTextLineSlot(fontPx, lineHeightMul, scale, pad, scrollY, asc, desc float64) RawTextSlot {
	slot := fontPx * lineHeightMul * scale
	return RawTextSlot{
		Slot:     slot,
		Baseline: (slot-(asc+desc))/2 + asc,
		Top0:     (pad - scrollY) * scale,
	}
}

// RawTextLineVisible reports whether a line whose slot starts at slotTop, in
// pane-local px with y down, overlaps the visible band [0, h).
func RawTextLineVisible(slotTop, slot, h float64) bool {
	return slotTop+slot > 0 && slotTop < h
}
