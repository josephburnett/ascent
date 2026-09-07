// Package gridpath resolves a pane's descent path to its leaf grid. It lives
// outside the wasm shim so it is unit-tested, because a stale-prefix slip sends
// you to the wrong grid.
package gridpath

// ResolveLeafGrid walks a descent path from rootGridID, following each well
// tile's child grid, and returns the grid id at the leaf. When lookup reports
// the current grid uncached or a path id absent from it, the walk returns the
// last good grid id, so a stale path prefix resolves as deep as it can and
// never past a gap. An empty rootGridID returns "".
//
// The wasm caller does the cache read inside lookup and may kick a background
// fetch on a miss.
func ResolveLeafGrid(rootGridID string, path []string,
	lookup func(gid, wellID string) (childGridID string, gridCached, tileFound bool)) string {
	if rootGridID == "" {
		return ""
	}
	gid := rootGridID
	for _, wellID := range path {
		child, cached, found := lookup(gid, wellID)
		if !cached || !found {
			return gid
		}
		gid = child
	}
	return gid
}
