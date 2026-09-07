// Package dragdrop turns cursor positions into grid cell coordinates and
// decides what a drag release does. DecideDrop is the one verdict both the
// in-flight preview and the commit obey.
package dragdrop

import "math"

// Pane is one pane's screen rectangle and viewport. CellPx is a cell's size
// at zoom 1.0, so a cell is CellPx*Zoom pixels on screen.
type Pane struct {
	ScreenX, ScreenY float64 // top-left of the pane in screen coordinates
	ScreenW, ScreenH float64
	Cx, Cy           float64 // viewport center in cells
	Zoom             float64
	CellPx           float64
}

// ScreenToCell converts screen coordinates to floating-point cells in the
// pane's viewport. The caller floors or rounds them.
func (p Pane) ScreenToCell(sx, sy float64) (float64, float64) {
	cellSize := p.CellPx * p.Zoom
	cx := p.Cx + (sx-(p.ScreenX+p.ScreenW/2))/cellSize
	cy := p.Cy + (sy-(p.ScreenY+p.ScreenH/2))/cellSize
	return cx, cy
}

// CellToScreen is the inverse of ScreenToCell.
func (p Pane) CellToScreen(cx, cy float64) (float64, float64) {
	cellSize := p.CellPx * p.Zoom
	sx := p.ScreenX + p.ScreenW/2 + (cx-p.Cx)*cellSize
	sy := p.ScreenY + p.ScreenH/2 + (cy-p.Cy)*cellSize
	return sx, sy
}

// CellAt is the integer cell containing a screen point, with floor
// semantics. See FloorCellAt.
func (p Pane) CellAt(sx, sy float64) (int64, int64) {
	cx, cy := p.ScreenToCell(sx, sy)
	return int64(math.Floor(cx)), int64(math.Floor(cy))
}

// SnapToCell rounds a floating-cell coordinate to the nearest whole cell,
// halves away from zero, so the snap is symmetric about zero. It answers
// where a dragged tile comes to rest; use FloorCellAt to ask which cell the
// cursor is inside.
func SnapToCell(c float64) int64 {
	if c >= 0 {
		return int64(c + 0.5)
	}
	return int64(c - 0.5)
}

// FloorCellAt is the integer cell containing the screen point (sx, sy) on a
// grid whose top-left is (originX, originY) and whose cells are cellSize
// pixels. Every interior point of cell N reports N, which is what a hit-test
// needs; SnapToCell rounds instead and would miss the lower-right half of
// every cell.
func FloorCellAt(originX, originY, cellSize, sx, sy float64) (int64, int64) {
	return int64(math.Floor((sx - originX) / cellSize)),
		int64(math.Floor((sy - originY) / cellSize))
}

// HiddenMatch reports whether a tile is skipped during render because it is
// being dragged, so the ghost and the cached row do not both show.
//
// It matches by tile id, the only identity a tile has. A clone is a different
// row that looks the same, so matching on anything a clone shares with its
// source would make every clone vanish during the drag.
func HiddenMatch(hiddenTileID string, hiddenPaneID, currentPaneID string, tileID string) bool {
	return hiddenTileID != "" && hiddenPaneID == currentPaneID && tileID == hiddenTileID
}

// ChildPreview is a well's child-grid preview as drawn inside its parent
// grid. Origin is the screen coordinate of child cell (0, 0) and CellPx is a
// child cell's size in screen pixels. ChildPreviewFor computes both.
type ChildPreview struct {
	OriginX, OriginY float64
	CellPx           float64
}

// ChildPreviewFor is the screen transform for a well's child-grid preview.
// ViewCx and ViewCy are the well's stored framing center in child cells, and
// previewRatio is the child cells per parent cell, which the caller resolves
// through zoomtrans.EffectiveViewZoom. The result is independent of pane
// size.
func ChildPreviewFor(parent Pane, well struct {
	X, Y, W, H     int64
	ViewCx, ViewCy float64
}, previewRatio float64) ChildPreview {
	parentCell := parent.CellPx * parent.Zoom
	previewCell := parentCell * previewRatio
	wellLeft, wellTop := parent.CellToScreen(float64(well.X), float64(well.Y))
	wellCenterX := wellLeft + float64(well.W)*parentCell/2
	wellCenterY := wellTop + float64(well.H)*parentCell/2
	return ChildPreview{
		OriginX: wellCenterX - well.ViewCx*previewCell,
		OriginY: wellCenterY - well.ViewCy*previewCell,
		CellPx:  previewCell,
	}
}

// ChildCellAtScreen is the floating child-grid cell at a screen point inside
// the preview.
func (cp ChildPreview) ChildCellAtScreen(sx, sy float64) (float64, float64) {
	return (sx - cp.OriginX) / cp.CellPx, (sy - cp.OriginY) / cp.CellPx
}

// CellToScreen is the screen coordinate of child cell (cx, cy)'s top-left
// corner in the preview.
func (cp ChildPreview) CellToScreen(cx, cy float64) (float64, float64) {
	return cp.OriginX + cx*cp.CellPx, cp.OriginY + cy*cp.CellPx
}

// TileContainsCell reports whether cell (cx, cy) lies within the rectangle
// (x, y, w, h).
func TileContainsCell(x, y, w, h, cx, cy int64) bool {
	return cx >= x && cx < x+w && cy >= y && cy < y+h
}

// RectsOverlap reports whether two cell-space footprints intersect. It is
// the predicate the server's overlap check applies, so the drop preflight and
// PlaceTile cannot disagree about a collision.
func RectsOverlap(ax, ay, aw, ah, bx, by, bw, bh int64) bool {
	return ax < bx+bw && bx < ax+aw && ay < by+bh && by < ay+ah
}

// InTileCenter reports whether a cell-space point lies in the inner third of
// the tile at (x, y, w, h). The region scales with the tile, so the copy and
// link grab handle feels the same at every zoom and on a 1x1 tile.
func InTileCenter(x, y, w, h int64, cellX, cellY float64) bool {
	xf, yf := float64(x), float64(y)
	wf, hf := float64(w), float64(h)
	return cellX >= xf+wf/3 && cellX <= xf+2*wf/3 &&
		cellY >= yf+hf/3 && cellY <= yf+2*hf/3
}

// ResizeAnchors is the cell state captured when a right-button tile resize
// starts. PinX and PinY are the corner diagonally opposite the click
// quadrant, OrigMovingX and OrigMovingY are the corner under the cursor, and
// ClickCellX and ClickCellY are the cursor's rounded cell, so a movement
// delta translates cell for cell.
type ResizeAnchors struct {
	PinX, PinY               int64
	OrigMovingX, OrigMovingY int64
	ClickCellX, ClickCellY   int64
}

// ResizeAnchorsFor is the anchors for a tile at (x, y, w, h) given the
// cursor's cell coordinates at click time. The click quadrant decides which
// corner is pinned.
func ResizeAnchorsFor(x, y, w, h int64, cellXf, cellYf float64) ResizeAnchors {
	var a ResizeAnchors
	midX := float64(x) + float64(w)/2
	midY := float64(y) + float64(h)/2
	if cellXf >= midX {
		a.PinX = x
		a.OrigMovingX = x + w
	} else {
		a.PinX = x + w
		a.OrigMovingX = x
	}
	if cellYf >= midY {
		a.PinY = y
		a.OrigMovingY = y + h
	} else {
		a.PinY = y + h
		a.OrigMovingY = y
	}
	a.ClickCellX = int64(math.Round(cellXf))
	a.ClickCellY = int64(math.Round(cellYf))
	return a
}

// ResizeFromCursor is the proposed (x, y, w, h) for the tile at the cursor's
// current rounded cell. Each side is at least 1.
func ResizeFromCursor(a ResizeAnchors, curCellX, curCellY int64) (int64, int64, int64, int64) {
	movX := a.OrigMovingX + (curCellX - a.ClickCellX)
	movY := a.OrigMovingY + (curCellY - a.ClickCellY)
	x, w := RangeFromAnchors(a.PinX, movX, a.OrigMovingX > a.PinX)
	y, h := RangeFromAnchors(a.PinY, movY, a.OrigMovingY > a.PinY)
	return x, y, w, h
}

// RangeFromAnchors is the start and length of a one-dimensional range
// between a pinned and a moving anchor, at least 1 long. When the two meet
// the range sits on the side the user first clicked, so the rectangle keeps
// its identity across the crossover.
func RangeFromAnchors(pin, moving int64, origRight bool) (start, length int64) {
	if moving == pin {
		if origRight {
			return pin, 1
		}
		return pin - 1, 1
	}
	if moving > pin {
		return pin, moving - pin
	}
	return moving, pin - moving
}

// MoveForbidden reports whether the server would reject a left-drag between
// grids when either end declares host_content. The fact is the owning
// plugin's declaration, carried on the grid.
//
// A drag across an id namespace is not a move at all, so crossPlugin exempts
// the host arms: it becomes a link, and TargetReadOnly gates a read-only
// destination separately. What stays forbidden is a same-namespace cross-grid
// move with a host-content end, because host content cannot migrate into
// Gridwell and host-side mv is not implemented. A same-grid move crosses no
// boundary.
func MoveForbidden(sameGrid, crossPlugin, srcHost, dstHost bool) bool {
	if sameGrid || crossPlugin {
		return false
	}
	return srcHost || dstHost
}

// Intent is what the armed gesture leaves at the destination. It is fixed by
// the press that arms the drag and never re-derived, so the preview and the
// commit cannot disagree with the gesture the user started.
type Intent int

const (
	// IntentMove is a left-drag, where the tile itself travels. Across an
	// id namespace it verdicts a link instead. It is the zero value, which
	// a palette template drag leaves unset.
	IntentMove Intent = iota
	// IntentCopy is a right-drag, an independent copy at the destination in
	// the same namespace or across one.
	IntentCopy
	// IntentLink is ctrl with a right-drag, where ctrl flips the right
	// button from copy to link. It links in whatever namespace the drop
	// lands in.
	IntentLink
)

// Creates reports whether the intent puts a new tile at the destination. A
// copy and a link both create, so the source stays put and is a neighbor the
// drop must not land on, a read-only destination refuses the arrival, and
// MoveForbidden does not apply.
func (i Intent) Creates() bool { return i != IntentMove }

// DropAction is the verdict for a drag release and its in-flight preview.
// The commit handlers and the ghost-preview handlers both route through
// DecideDrop, so the ghost and the outcome cannot drift apart.
type DropAction int

const (
	// DropNavigate is a bare click on an already-focused pane, which
	// descends, ascends or selects. It places nothing.
	DropNavigate DropAction = iota
	// DropNavigateSplit is that bare click with ctrl held at press time, so
	// a descent lands in a new split pane. Every other outcome of the click
	// is unchanged, and on an unfocused pane the click is still focus-only.
	DropNavigateSplit
	// DropFocusOnly is a bare click on a pane that was unfocused at press
	// time, which only moved focus.
	DropFocusOnly
	// DropCreateTemplate creates a fresh tile at the snapped cell from a
	// palette swatch.
	DropCreateTemplate
	// DropPanEnd is an empty-space drag, which persists the viewport.
	DropPanEnd
	// DropDelete is a release over the source pane's trashcan button.
	DropDelete
	// DropRejected snaps back.
	DropRejected
	// DropMove relocates the tile.
	DropMove
	// DropClone copies it.
	DropClone
	// DropLink creates a reference at the destination, an exit well for a
	// grid and a leaf link otherwise. Two gestures verdict it: ctrl with a
	// right-drag, and a left-drag whose ends are in different id namespaces,
	// where there is no cross-plugin move. Either way the source is
	// untouched and identity never migrates.
	DropLink
)

// DropInput is every world-read a drop decision needs, gathered once at
// release or per preview frame before any teardown clears the drag state. It
// holds no App fields and no js.Value, so a cleared field can never be read
// late. The caller resolves each field; Forbidden comes from MoveForbidden
// and is false for a copy or a link, and Occupied excludes the moving tile on
// a move, mirroring the server's PlaceTile.
type DropInput struct {
	Started bool
	// OriginFocused means the origin pane was already focused when the
	// press landed. A bare click on an unfocused pane is focus-only,
	// whatever tile sits under the cursor, and the + button and the corner
	// circle follow the same rule.
	OriginFocused bool
	// SplitNav means ctrl was held at left-press time. It is fixed at
	// press, so releasing ctrl mid-click cannot change the verdict, and
	// only the bare-click arm reads it. Touch synthesizes no ctrlKey.
	SplitNav   bool
	IsTemplate bool
	// Intent is the armed gesture's meaning, read from the drag state.
	Intent     Intent
	TileID     string // "" for a pan or empty-space drag
	OverDelete bool
	HasTarget  bool
	Forbidden  bool
	// TargetReadOnly means the destination grid refuses creation, so an
	// arrival is rejected before an RPC the server would refuse. A
	// same-grid left-drag is placement rather than creation and is exempt.
	TargetReadOnly bool
	// SameGrid means the drop lands in the tile's own grid, so it is a
	// rearrangement and arrives nowhere.
	SameGrid bool
	SameCell bool
	Occupied bool
	// CrossPlugin means the source and target grids live in different id
	// namespaces. A left-drag then verdicts DropLink; a right-drag stays
	// DropClone and the server copies.
	CrossPlugin bool
}

// DecideDrop maps a gathered DropInput to the action both the preview and
// the commit obey. The branch order is the decision: an earlier arm wins.
//
// The HasTarget check sits above every arm that lands something in a grid,
// creation included, so no arm commits against a destination the target
// resolution refused. Only the two arms that land in no grid stand above it:
// a pan, which ends wherever it ends, and the trashcan, which resolves
// against the bar's own button. A template carries no tile id, so OverDelete
// is false and it never reaches the delete arm.
func DecideDrop(in DropInput) DropAction {
	switch {
	case !in.Started && !in.OriginFocused:
		return DropFocusOnly
	case !in.Started && in.SplitNav:
		return DropNavigateSplit
	case !in.Started:
		return DropNavigate
	case in.TileID == "" && !in.IsTemplate:
		return DropPanEnd
	case in.OverDelete:
		return DropDelete
	case !in.HasTarget:
		return DropRejected
	case in.IsTemplate:
		return DropCreateTemplate
	case in.Forbidden:
		return DropRejected
	case in.TargetReadOnly && !(in.SameGrid && !in.Intent.Creates()):
		// Read-only gates arrivals. A same-grid left-drag is placement,
		// which a read-only projection accepts and persists.
		return DropRejected
	case in.SameCell:
		return DropRejected
	case in.Occupied:
		return DropRejected
	case in.Intent == IntentCopy:
		return DropClone
	case in.Intent == IntentLink || in.CrossPlugin:
		// Ctrl asked for a link, and a move across an id namespace has no
		// other answer. Both land on the one link commit, so they cannot
		// produce two different kinds of reference.
		return DropLink
	default:
		return DropMove
	}
}

// GhostPlan is how the in-flight drag ghost renders for a drop verdict.
type GhostPlan struct {
	PaneID         string  // pane whose coordinate space the ghost rests in
	TargetCellSize float64 // size the ghost lerps toward
	Fragmentation  float64 // 1 shatters into the trashcan
	Forbidden      bool    // draw the no-entry badge
	// Link draws the dashed ghost and chain badge, so the user learns
	// mid-drag that the source stays put and the destination gains a
	// reference. Without it a cross-plugin left-drag would look like a move
	// and the source's survival would read as a surprise duplicate.
	Link   bool
	Cursor string // CSS cursor: "" or "not-allowed"
}

// GhostPlanForDrop maps a DecideDrop verdict and its reject cause to the
// ghost styling. The verdict already carries the intent, so it is not passed
// again. The ghost rests in a different pane per verdict, which is why both
// pane ids and both cell sizes come in: the origin pane for a delete or an
// off-canvas reject, the target pane for a placement or a forbidden
// cross-grid move.
//
// SameCell and Occupied get no style of their own. The preview is optimistic
// about placement and shows the snap-to-cell, and the commit does the
// authoritative overlap check.
func GhostPlanForDrop(action DropAction, forbidden bool,
	originPaneID, targetPaneID string, srcCellSize, targetCellSize float64) GhostPlan {
	switch action {
	case DropDelete:
		return GhostPlan{PaneID: originPaneID, TargetCellSize: srcCellSize * 0.2, Fragmentation: 1.0}
	case DropLink:
		return GhostPlan{PaneID: targetPaneID, TargetCellSize: targetCellSize, Link: true}
	case DropRejected:
		if forbidden {
			return GhostPlan{PaneID: targetPaneID, TargetCellSize: srcCellSize, Forbidden: true, Cursor: "not-allowed"}
		}
		return GhostPlan{PaneID: originPaneID, TargetCellSize: srcCellSize}
	default: // DropMove / DropClone
		return GhostPlan{PaneID: targetPaneID, TargetCellSize: targetCellSize}
	}
}

// PromoteToWell reports whether the tile under the cursor promotes the drop
// target to its own child grid. The tile must be an enterable well and must
// not be the dragged tile, since dropping a well into its own subtree would
// make a cycle the server rejects. The caller resolves isWell from
// rpc.IsWellKind, which keeps this package free of api/rpc.
func PromoteToWell(isWell bool, childGridID, tileID, draggedTileID string) bool {
	return isWell && childGridID != "" && tileID != draggedTileID
}
