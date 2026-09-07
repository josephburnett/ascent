// Package panepreview computes the geometry of a pane tile's mini-render, the
// stored pane layout drawn small inside the tile's rect. It is pure Go, so the
// continuity between preview and descent is provable headlessly.
//
// The preview is the live workspace shrunk uniformly by Scale. pane.Layout is
// affine in its root rect, so each leaf's preview rect is its live rect under
// that scaling and each leaf's content cell size is its live cell size times
// the same factor. Descending into the pane tile lands on what the preview
// showed, only bigger.
package panepreview

import "github.com/josephburnett/gridwell/client/pane"

// Leaf is one pane of the mini-render: the leaf, its rect inside the tile, and
// the cell size its grid content draws at.
type Leaf struct {
	Pane *pane.Pane
	Rect pane.Rect
	// PreviewCell is the on-screen size of one grid cell inside this leaf, the
	// live cell size (Zoom × CellPx) shrunk by the tile scale.
	PreviewCell float64
}

// Scale returns the factor by which the live workspace shrinks into the tile
// rect, the smaller of the width and height ratios so the layout fits without
// distortion. A degenerate live rect returns zero.
func Scale(tileRect, liveRootRect pane.Rect) float64 {
	if liveRootRect.W <= 0 || liveRootRect.H <= 0 {
		return 0
	}
	sx := tileRect.W / liveRootRect.W
	sy := tileRect.H / liveRootRect.H
	if sx < sy {
		return sx
	}
	return sy
}

// Leaves lays the tree out into the tile rect and pairs each leaf with its
// preview transform. pane.Layout honors Zoomed, giving a zoomed pane the whole
// rect, so the mini-render shows what descent would restore.
func Leaves(t *pane.Tree, tileRect pane.Rect, scale float64) []Leaf {
	rects := pane.Layout(t, tileRect)
	var out []Leaf
	t.Walk(func(p *pane.Pane) {
		r, ok := rects[p.ID]
		if !ok {
			return // when a pane is zoomed only that leaf has a rect
		}
		out = append(out, Leaf{
			Pane:        p,
			Rect:        r,
			PreviewCell: p.Zoom * pane.CellPx * scale,
		})
	})
	return out
}
