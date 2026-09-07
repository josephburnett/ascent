// Package wsbar owns the bottom bar's geometry: the one bar at the bottom of
// the window, carrying the circle slot and the nav chain, which is the complete
// path from the root as square tile previews, pane-tile boundaries included.
// Render and input read the same segment rects, so the crumb you see is the
// crumb you hit.
//
// The bar is always present, because it is the one home for where the user is.
// Its band, the full-width RowH row at the bottom, is reserved layout, and the
// pane tree ends at that row's top edge (Band), so no pane and no native
// surface sized from a pane can paint over it. The band's height never depends
// on which pane has focus; only the bar's own chrome moves, riding the focused
// pane's span (Rect).
package wsbar

// Band divides the window's vertical space: the pane tree gets everything above
// the band, the band is the RowH strip below it, and the caller's notice strip
// (stripH) keeps the bottom. paneH is the pane tree's height and also the band's
// top edge, since panes start at y=0, so layout and bar cannot disagree about
// where they meet. ok=false when what is left cannot hold the band, and the
// panes then take it all.
func Band(winH, stripH float64) (paneH float64, ok bool) {
	avail := winH - stripH
	if avail < 0 {
		avail = 0
	}
	if avail < RowH {
		return avail, false
	}
	return avail - RowH, true
}

// Rect is the one answer to where the bar is: render, hit-test and the rename
// inputs all read it, so the chrome you see is the chrome you hit. Vertically it
// is the RowH row at the band's top edge (Band), the same reservation whichever
// pane has focus, so panes never resize as focus moves. Horizontally it is only
// the focused pane's span (paneX, paneW), centered under that pane, so the bar
// reads as a tab under the pane in use and the circle slot is never a wide
// screen away. A span wider than the window, or one hanging off an edge, is
// clamped whole into the window.
//
// ok=false when the window cannot hold the band, or when there is no pane for
// the bar to sit under; the band's row is then plain background all the way
// across.
func Rect(winW, winH, stripH, paneX, paneW float64) (x, top, w float64, ok bool) {
	top, ok = Band(winH, stripH)
	if !ok || winW <= 0 || paneW <= 0 {
		return 0, 0, 0, false
	}
	w = paneW
	if w > winW {
		w = winW
	}
	x = paneX + (paneW-w)/2 // centered under the pane when the window clips it
	if x+w > winW {
		x = winW - w
	}
	if x < 0 {
		x = 0
	}
	return x, top, w, true
}

// Zone says where a point falls with respect to the bar.
type Zone int

const (
	// ZoneOutside: not in the band's row, so the point belongs to a pane or to
	// the notice strip below.
	ZoneOutside Zone = iota
	// ZoneBand: in the band's row but off the bar. Nothing is under the band, so
	// a point here is swallowed rather than passed on to a pane.
	ZoneBand
	// ZoneBar: on the bar's own chrome, where every bar gesture lives.
	ZoneBar
)

// Where classifies a point against the rect Rect returned.
func Where(px, py, x, top, w float64) Zone {
	if py < top || py >= top+RowH {
		return ZoneOutside
	}
	if px < x || px >= x+w {
		return ZoneBand
	}
	return ZoneBar
}

// RowH is the bar's height in CSS px. 32 keeps the band thin while a
// square chain crumb (side RowH) stays legible as a preview.
const RowH = 32.0

// SlotW is the width reserved at the bar's right end for the circle button
// slot. It sits in the band, so it never obscures content and needs no native
// overlay over live views. Layout never places a crumb inside it.
const SlotW = 48.0

// Segment is one crumb's hit and draw rect, relative to the bar's left edge.
// Index is the position in the caller's full crumb list, so under
// left-truncation the visible segments are a suffix that still points at the
// right crumbs.
type Segment struct {
	Index int
	X, W  float64
}

// BoundaryW is a pane-tile boundary crumb's width. The wide light-blue bar
// stands out from the RowH squares of chain crumbs, and it is the rename
// target.
const BoundaryW = 120.0

// Layout lays the one nav chain, the complete path from the root, left to
// right; widths[i] gives each crumb its width, RowH for a chain crumb and
// BoundaryW for a pane-tile boundary. When the band cannot fit them all, crumbs
// drop from the left, so the tail keeps priority and the survivors keep full
// size, because a too-small preview reads as nothing. The current pane's name
// is a separate centered title rather than a crumb.
func Layout(widths []float64, width float64) []Segment {
	if len(widths) == 0 || width <= 0 {
		return nil
	}
	width -= SlotW // the right-end circle slot is reserved
	first := len(widths)
	rem := width
	for i := len(widths) - 1; i >= 0; i-- {
		if widths[i] > rem {
			break
		}
		rem -= widths[i]
		first = i
	}
	if first == len(widths) {
		return nil
	}
	out := make([]Segment, 0, len(widths)-first)
	x := 0.0
	for i := first; i < len(widths); i++ {
		out = append(out, Segment{Index: i, X: x, W: widths[i]})
		x += widths[i]
	}
	return out
}

// titlePad is the room on either side of the centered title.
const titlePad = 8.0

// minTitleW is the narrowest span worth drawing a title into.
const minTitleW = 24.0

// TitleSpan centers the pane title in the free space between the crumbs' end
// and the circle slot, so growing crumbs cannot crowd it one-sidedly. crumbsEnd
// is the right edge of the last crumb, 0 with none, and textW is the measured
// title width including padding. x is relative to the band's left edge, a title
// wider than the free space is clamped to it, and ok=false when less than
// minTitleW remains.
func TitleSpan(crumbsEnd, width, textW float64) (x, w float64, ok bool) {
	left := crumbsEnd + titlePad
	right := width - SlotW - titlePad
	if right-left < minTitleW {
		return 0, 0, false
	}
	w = textW
	if w > right-left {
		w = right - left
	}
	return left + (right-left-w)/2, w, true
}

// At returns the segment under x (relative to the bar's left edge), or
// ok=false when x falls outside every crumb.
func At(segs []Segment, x float64) (Segment, bool) {
	for _, s := range segs {
		if x >= s.X && x < s.X+s.W {
			return s, true
		}
	}
	return Segment{}, false
}

// SegmentAt returns the visible segment for a full-list index, or ok=false when
// it was truncated away. The inline rename input is placed over it.
func SegmentAt(segs []Segment, index int) (Segment, bool) {
	for _, s := range segs {
		if s.Index == index {
			return s, true
		}
	}
	return Segment{}, false
}
