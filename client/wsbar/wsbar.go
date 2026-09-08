// Package wsbar owns the bottom bar's geometry: the circle slot and the nav
// chain, the complete path from the root as square tile previews. Render and
// input read the same segment rects, so the crumb you see is the crumb you
// hit. The band, the full-width RowH row at the bottom, is reserved layout and
// the pane tree ends at its top edge (Band), so nothing sized from a pane can
// paint over it. Only the bar's own chrome moves with focus (Rect).
package wsbar

// Band divides the window's vertical space: panes above, the RowH band, then
// the caller's notice strip (stripH). paneH is both the pane tree's height and
// the band's top edge, so layout and bar cannot disagree about where they
// meet. ok=false when what is left cannot hold the band.
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
// inputs all read it. Vertically it is the band's row whichever pane has
// focus, so panes never resize as focus moves. Horizontally it is the focused
// pane's span, so the bar reads as a tab under the pane in use; a span wider
// than the window is clamped whole into it. ok=false leaves the band plain
// background all the way across.
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

type Zone int

const (
	// ZoneOutside belongs to a pane or to the notice strip below.
	ZoneOutside Zone = iota
	// ZoneBand is the band's row off the bar. Nothing is under the band, so a
	// point here is swallowed rather than passed to a pane.
	ZoneBand
	// ZoneBar is the bar's own chrome, where every bar gesture lives.
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

// RowH keeps the band thin while a square chain crumb stays legible as a
// preview.
const RowH = 32.0

// SlotW is reserved at the bar's right end for the circle button. It sits in
// the band, so it never obscures content, and Layout never places a crumb in
// it.
const SlotW = 48.0

// Segment is one crumb's hit and draw rect, relative to the bar's left edge.
// Index is the position in the caller's full list, so under left-truncation
// the visible segments still point at the right crumbs.
type Segment struct {
	Index int
	X, W  float64
}

// BoundaryW makes a pane-tile boundary crumb stand out from the RowH squares
// of chain crumbs. It is the rename target.
const BoundaryW = 120.0

// Layout lays the nav chain left to right; widths[i] is RowH for a chain crumb
// and BoundaryW for a pane-tile boundary. When the band cannot fit them all,
// crumbs drop from the left and the survivors keep full size, a too-small
// preview reading as nothing. The current pane's name is a separate centered
// title, not a crumb.
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

const titlePad = 8.0

const minTitleW = 24.0

// TitleSpan centers the pane title between the crumbs' end and the circle
// slot, so growing crumbs cannot crowd it one-sidedly. crumbsEnd is 0 with no
// crumbs and textW includes padding. x is relative to the band's left edge and
// ok=false when less than minTitleW remains.
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

// At returns the segment under x, relative to the bar's left edge.
func At(segs []Segment, x float64) (Segment, bool) {
	for _, s := range segs {
		if x >= s.X && x < s.X+s.W {
			return s, true
		}
	}
	return Segment{}, false
}

// SegmentAt returns the visible segment for a full-list index; ok=false when
// it was truncated away. The inline rename input is placed over it.
func SegmentAt(segs []Segment, index int) (Segment, bool) {
	for _, s := range segs {
		if s.Index == index {
			return s, true
		}
	}
	return Segment{}, false
}
