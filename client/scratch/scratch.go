// Package scratch answers where a pane's ephemeral visits live and whether a
// tile is one of them. An ephemeral visit, such as a url typed into the + menu
// or a shell opened from it, is a real row in the scratch grid, which no pane
// stands on.
//
// The serving node stamps that grid onto every grid it answers, as
// Grid.scratch_grid_id chained through mounts, and this package reads only that
// stamp. The answer cannot be guessed from an id: a mounted remote grid's first
// segment is the local node, so a roster lookup keyed on it returns the local
// node's scratch grid for a grid that lives elsewhere, and every reader then
// decides about the wrong node.
//
// The answer is therefore three-valued, and not known yet is one of the three.
// When the grid is not cached the caller is told so instead of handed a guess.
package scratch

// Grid is the part of a grid this rule reads: whether the row is cached, and
// the scratch grid the serving node stamped on it. An uncached grid carries no
// stamp to read, which is why Cached is a field rather than an empty
// ScratchGridID.
type Grid struct {
	Cached        bool
	ScratchGridID string
}

// For returns the scratch grid that ephemeral visits from g land in, and
// whether that is known. An uncached grid answers ("", false), meaning not
// known yet rather than a guess. A cached grid whose node stamped none answers
// ("", true), a known answer of nowhere.
func For(g Grid) (id string, known bool) {
	if !g.Cached {
		return "", false
	}
	return g.ScratchGridID, true
}

// Ephemeral reports whether a tile living in tileGridID is an ephemeral visit
// from a pane standing on g, and whether that is known. Unknown is not false. A
// caller that acts on an ephemeral tile, deleting it on ascent or promoting it
// onto a grid, needs a known yes, and a caller about to write something durable
// about it needs a known no.
func Ephemeral(g Grid, tileGridID string) (ephemeral, known bool) {
	id, known := For(g)
	if !known {
		return false, false
	}
	return id != "" && tileGridID == id, true
}
