// Package gesture classifies a right-button-down into a Kind and resolves the
// gestures whose release is geometry. The App resolves the facts and hands
// them in as an Input; a gesture whose release is a drop stays in the App.
package gesture

import "github.com/josephburnett/gridwell/client/pane"

// Kind is fixed at right-button-down and never changes mid-gesture.
type Kind int

const (
	None Kind = iota
	// Ascend is armed on the corner circle. Release inside it ascends,
	// dragging out cancels.
	Ascend
	// TileCenter is the copy and link handle, a tile's inner third. A drag
	// past the threshold clones, or links when ctrl was held at the press.
	TileCenter
	// TileResize rubber-bands the footprint from the opposite corner.
	TileResize
	Swap
	// Split splits along the armed side at the release ratio. The right
	// button splits from a border wherever it starts; resizing and closing
	// under pressure belong to the left button.
	Split
)

// Input is the facts resolved at right-button-down. Classify reads them in
// priority order, so a field matters only when every higher one is false.
type Input struct {
	// InGridView is the only place a tile gesture is valid.
	InGridView   bool
	OverTile     bool
	InTileCenter bool
	Region       pane.Region
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
		return Split
	case in.Region.IsSwap():
		return Swap
	case in.Region.IsSplit():
		return Split
	}
	return None
}

// SplitOutcome resolves a Split release into its final ratio, the side already
// resolved by SplitSideFromDrag. The cursor must land where both children keep
// pane.MinPanePx; ok is false for a silent cancel.
func SplitOutcome(side pane.Side, paneRect pane.Rect, curX, curY float64) (ratio float64, ok bool) {
	pos, ok := pane.SplitClampedPosition(side, paneRect, curX, curY)
	if !ok {
		return 0, false
	}
	return pane.SplitRatioFromPos(side, paneRect, pos), true
}

// SplitSideFromDrag reads the drag rather than the grab, so either side of a
// border behaves identically and the direction can flip mid-gesture. The new
// pane opens between the grabbed border and the cursor. active is false until
// the drag clears SplitArmPx, so jitter commits nothing.
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

// SplitArmPx keeps a bare right-click on a border from splitting.
const SplitArmPx = 8.0

// ResizeAffordance says whether a left drag would arm a pane-boundary resize,
// and which CSS cursor advertises it. The hover path and the arm path both
// call it, so the cursor appears exactly where a drag would resize. g is
// pane.GrabDividers' answer; a grab on both axes is a corner.
func ResizeAffordance(g pane.DividerGrab) (arm bool, cursor string) {
	if !g.Any() {
		return false, ""
	}
	switch {
	case g.Both():
		// Top-left and bottom-right run NW to SE, the other two NE to SW.
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
