// Package palette computes the layout of the tile-creation menu: the popover
// over a pane's + button holding the swatches the user drags onto the canvas.
// One swatch per declared doorway sits on the top row (Doorways), the tile
// primitives on a row below, and a disclosure strip folds the top row away
// (Show). Layout is pure, depending only on the + button's center and the
// tile counts; the wasm renderer reads the rects and paints into them.
package palette

import "github.com/josephburnett/gridwell/client/pane"

// Rect is pane.Rect, the client's one screen-space rectangle type.
type Rect = pane.Rect

// Config is the tunable layout for the + button and the popover.
type Config struct {
	// PlusRadius is the + button's radius, hit-test and visual. It fits
	// inside the bottom bar's band (wsbar.RowH) with margin.
	PlusRadius float64
	// TileMinPx and TileMaxPx clamp the per-tile size in screen pixels.
	TileMinPx, TileMaxPx float64
	// GapPx is the gutter between tiles and around the popover border.
	GapPx float64
	// CellPx is the renderer's base cell size at zoom 1.0. TilePx is a
	// fixed fraction of it.
	CellPx float64
	// ToggleH is the height of the disclosure strip, a band the popover's
	// full width. The strip is a control the user presses, so it is not
	// sized as a swatch.
	ToggleH float64
}

// Default returns the constants the wasm renderer uses, so a test pins the
// values the user sees.
func Default() Config {
	return Config{
		PlusRadius: 14,
		TileMinPx:  48,
		TileMaxPx:  128,
		GapPx:      8,
		CellPx:     pane.CellPx,
		ToggleH:    18,
	}
}

// Layout snapshots one palette's input. Every method is pure.
type Layout struct {
	Cfg Config
	// PlusX and PlusY are the + button's center, the bottom bar's
	// right-end slot.
	PlusX, PlusY float64
	NumTiles     int
	// TopRow is how many of NumTiles sit in the popover's first row; the
	// primitives take the row below. Either count may be zero, and then the
	// populated row is the only one.
	TopRow int
	// Toggle is whether the disclosure strip is in the popover; Show
	// decides. The strip sits above the primitives in either fold state.
	Toggle bool
}

// topCount and bottomCount split NumTiles across the two popover rows.
func (l Layout) topCount() int {
	if l.TopRow <= 0 {
		return 0
	}
	return min(l.TopRow, l.NumTiles)
}

func (l Layout) bottomCount() int { return l.NumTiles - l.topCount() }

func (l Layout) rowCount() int {
	n := 0
	if l.topCount() > 0 {
		n++
	}
	if l.bottomCount() > 0 {
		n++
	}
	return n
}

// toggleRow is the row index the strip sits above. Rows from there down are
// pushed by the strip's band.
func (l Layout) toggleRow() int {
	if l.topCount() > 0 {
		return 1
	}
	return 0
}

// toggleBand is the vertical space the strip takes.
func (l Layout) toggleBand() float64 {
	if !l.Toggle {
		return 0
	}
	return l.Cfg.ToggleH + l.Cfg.GapPx
}

// rowY is the top of a popover row, counting the strip's band for every row
// at or below it.
func (l Layout) rowY(row int) float64 {
	y := l.PopoverRect().Y + l.Cfg.GapPx + float64(row)*(l.TilePx()+l.Cfg.GapPx)
	if row >= l.toggleRow() {
		y += l.toggleBand()
	}
	return y
}

// rowWidthPx is the popover width a row of n tiles needs.
func (l Layout) rowWidthPx(n int) float64 {
	return float64(n)*l.TilePx() + float64(n+1)*l.Cfg.GapPx
}

// PlusCenter returns the screen-space center of the + button.
func (l Layout) PlusCenter() (cx, cy float64) {
	return l.PlusX, l.PlusY
}

// TilePx is the per-tile size in screen pixels, three quarters of a default
// cell and independent of pane zoom, so the menu is the same size wherever
// it opens. The drag ghost resizes to the destination zoom on drop, as it
// does when a tile is dragged across wells.
func (l Layout) TilePx() float64 {
	return l.Cfg.CellPx * 0.75
}

// PopoverRect is the screen rect of the whole popover, anchored above the +
// button and sized to the rows it holds.
func (l Layout) PopoverRect() Rect {
	tile := l.TilePx()
	w := max(l.rowWidthPx(l.topCount()), l.rowWidthPx(l.bottomCount()))
	rows := float64(l.rowCount())
	h := rows*tile + (rows+1)*l.Cfg.GapPx + l.toggleBand()
	cx, cy := l.PlusCenter()
	x := cx + l.Cfg.PlusRadius - w
	y := cy - l.Cfg.PlusRadius - h - 8
	return Rect{X: x, Y: y, W: w, H: h}
}

// TileRect is the screen rect of the i'th swatch. Each row is centered in
// the popover, so a short row sits under the middle of a wider one.
func (l Layout) TileRect(i int) Rect {
	pop := l.PopoverRect()
	tile := l.TilePx()
	gap := l.Cfg.GapPx
	top := l.topCount()
	row, col, count := 0, i, top
	if i >= top {
		// The primitives take the second row only when the row above it
		// is populated.
		row, col, count = l.toggleRow(), i-top, l.bottomCount()
	}
	rowX := pop.X + (pop.W-l.rowWidthPx(count))/2
	return Rect{
		X: rowX + gap + float64(col)*(tile+gap),
		Y: l.rowY(row),
		W: tile,
		H: tile,
	}
}

// ToggleRect is the screen rect of the disclosure strip, or the zero rect
// when the popover has no toggle.
func (l Layout) ToggleRect() Rect {
	if !l.Toggle {
		return Rect{}
	}
	pop := l.PopoverRect()
	gap := l.Cfg.GapPx
	return Rect{
		X: pop.X + gap,
		Y: l.rowY(l.toggleRow()) - l.toggleBand(),
		W: pop.W - 2*gap,
		H: l.Cfg.ToggleH,
	}
}

// PointInToggle reports whether (x, y) is on the disclosure strip. It is
// false when there is no toggle, so a caller needs no second guard.
func (l Layout) PointInToggle(x, y float64) bool {
	if !l.Toggle {
		return false
	}
	r := l.ToggleRect()
	return x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H
}

// TileIndexAt is the index of the swatch under (x, y), or -1 for a gutter or
// a point outside the popover.
func (l Layout) TileIndexAt(x, y float64) int {
	for i := range l.NumTiles {
		r := l.TileRect(i)
		if x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H {
			return i
		}
	}
	return -1
}

// PointInPopover reports whether (x, y) is inside the popover rect. A click
// that misses a swatch but lands here keeps the menu open.
func (l Layout) PointInPopover(x, y float64) bool {
	r := l.PopoverRect()
	return x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H
}
