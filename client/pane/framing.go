package pane

import "slices"

// The framing writeback and the liveness projection. Both are projections of
// the frame stack, so neither can drift from where the pane is.

// FramingOwner names the row that owns the settled framing of a pane's place,
// the one question every ascent and settle tick asks.
type FramingOwner struct {
	// Content: the place is a content tile, so what settles is its text
	// scroll, not grid framing.
	Content bool
	// TileID is the doorway the pane came in through, or the content tile.
	// Empty at a root grid with no doorway.
	TileID     string
	DoorAnchor string // the doorway's own grid, one level out
	DoorPath   []string
	// RootGridID owns the framing when there is no doorway. Always set for a
	// grid place, so a caller whose doorway lookup misses, as a + menu
	// portal's does, falls back without a second rule.
	RootGridID string
}

// FramingTarget is the row that owns the pane's framing: the doorway it came
// in by, or the grid itself at a root.
func (s *Stack) FramingTarget() FramingOwner {
	if s.Content {
		return FramingOwner{Content: true, TileID: s.Door}
	}
	anchor, path := s.AnchorPathAt(len(s.below))
	own := FramingOwner{RootGridID: anchor}
	if s.Door == "" {
		return own
	}
	own.TileID = s.Door
	if len(path) > 0 {
		own.DoorAnchor, own.DoorPath = anchor, slices.Clone(path[:len(path)-1])
	} else {
		own.DoorAnchor, own.DoorPath = s.AnchorPathAt(len(s.below) - 1)
	}
	return own
}

// PaneGrid names one pane and the grid it shows.
type PaneGrid struct {
	PaneID string
	GridID string
}

// FramingWriters applies the one-active-surface rule to grid framing: of
// several panes showing one grid only the focused one writes, because every
// sibling writing its own rect-derived values each settle tick thrashes the
// persisted framing.
func FramingWriters(panes []PaneGrid, focusedID string) map[string]bool {
	byGrid := map[string]int{}
	for _, p := range panes {
		byGrid[p.GridID]++
	}
	out := map[string]bool{}
	for _, p := range panes {
		out[p.PaneID] = byGrid[p.GridID] == 1 || p.PaneID == focusedID
	}
	return out
}

// Holder names a pane and the content tile it is descended into.
type Holder struct {
	PaneID string
	TileID string
}

// TakeOver applies one live surface per content tile: opening tileID in
// openerID freezes every other pane's surface on the same content, at any
// stack level, and returns those panes. The opener is never in the list, so a
// keep-alive return is idempotent.
func TakeOver(holders []Holder, openerID, tileID string) []string {
	var out []string
	for _, h := range holders {
		if h.TileID == tileID && h.PaneID != openerID {
			out = append(out, h.PaneID)
		}
	}
	return out
}
