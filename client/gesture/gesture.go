// Package gesture classifies a right-button-down into a Kind and resolves
// the gestures whose release is geometry. The App looks up the facts, such as
// whether the cursor is over a tile, and hands them in as an Input; Classify
// orders them. A gesture whose release is a drop resolution stays in the
// App.
package gesture

import "github.com/josephburnett/gridwell/client/pane"

// Kind classifies an in-flight right-button gesture. It is fixed at
// right-button-down and never changes mid-gesture. None means the down armed
// nothing.
type Kind int

const (
	None Kind = iota
	// Ascend is armed on the corner circle when the pane has somewhere to
	// ascend to. Release inside the circle ascends and dragging out
	// cancels.
	Ascend
	// TileCenter is the copy and link grab handle, armed in a tile's inner
	// third. A drag past the threshold clones the tile, or links it when
	// ctrl was held at the press, and a bare release does nothing.
	TileCenter
	// TileResize is armed on a tile outside its center. It rubber-bands the
	// footprint from the diagonally opposite corner.
	TileResize
	// Swap exchanges the origin pane with the pane under the cursor at
	// release.
	Swap
	// Split splits the pane along the armed side at the release ratio. The
	// right button splits from a border wherever it starts; resizing and
	// closing under pressure belong to the left button.
	Split
)

// Input is the facts resolved at right-button-down. Classify reads them in
// priority order, so a field matters only when every higher one is false.
type Input struct {
	// InGridView is true when the pane shows a grid, which is the only
	// place a tile gesture is valid. InTileCenter distinguishes the inner
	// third, the copy and link handle, from the resize ring.
	InGridView   bool
	OverTile     bool
	InTileCenter bool

	// Region is the pane sub-region under the cursor.
	Region pane.Region
}

// Classify maps the resolved facts to a Kind. A tile under the cursor claims
// the down first, and only then does the pane sub-region decide.
func Classify(in Input) Kind {
	switch {
	case in.InGridView && in.OverTile:
		if in.InTileCenter {
			return TileCenter
		}
		return TileResize
	}
	switch {
	case in.Region.IsResize():
		// A border right-drag is a split wherever it starts, over a
		// divider exactly as at a screen edge.
		return Split
	case in.Region.IsSwap():
		return Swap
	case in.Region.IsSplit():
		return Split
	}
	return None
}

// SplitOutcome resolves a Split release into the final ratio for the pane
// the cursor is in, with the side already resolved by SplitSideFromDrag. The
// cursor must land where both children keep pane.MinPanePx; ok is false for a
// silent cancel.
func SplitOutcome(side pane.Side, paneRect pane.Rect, curX, curY float64) (ratio float64, ok bool) {
	pos, ok := pane.SplitClampedPosition(side, paneRect, curX, curY)
	if !ok {
		return 0, false
	}
	return pane.SplitRatioFromPos(side, paneRect, pos), true
}

// SplitSideFromDrag resolves a right-drag split's side from the drag rather
// than the grab, so either side of a border behaves identically and the
// direction can flip mid-gesture. The new pane opens in the space between the
// grabbed border and the cursor. active is false until the drag clears
// SplitArmPx, so a bare click or jitter commits nothing.
func SplitSideFromDrag(axis pane.Direction, startX, startY, curX, curY float64) (side pane.Side, active bool) {
	d := curX - startX
	if axis == pane.Horizontal {
		d = curY - startY
	}
	if d > SplitArmPx {
		if axis == pane.Horizontal {
			return pane.SideTop, true
		}
		return pane.SideLeft, true
	}
	if d < -SplitArmPx {
		if axis == pane.Horizontal {
			return pane.SideBottom, true
		}
		return pane.SideRight, true
	}
	return 0, false
}

// SplitArmPx is the drag distance that arms a split, so a bare right-click
// on a border never splits.
const SplitArmPx = 8.0

// ResizeAffordance says whether a left drag would arm a pane-boundary resize
// at the cursor, and which CSS cursor advertises it. The hover path and the
// arm path both call it, so the resize cursor appears exactly where a drag
// would resize.
//
// g is which dividers the press is close enough to take, at most one per axis
// (pane.GrabDividers). A grab on both axes is a corner, and the cursor names
// the diagonal it opens along.
func ResizeAffordance(g pane.DividerGrab) (arm bool, cursor string) {
	if !g.Any() {
		return false, ""
	}
	switch {
	case g.Both():
		// The top-left and bottom-right corners run NW to SE, the other
		// two NE to SW.
		if (g.HorizSide == pane.SideTop) == (g.VertSide == pane.SideLeft) {
			return true, "nwse-resize"
		}
		return true, "nesw-resize"
	case g.HasVert:
		return true, "ew-resize"
	default: // horizontal only
		return true, "ns-resize"
	}
}
