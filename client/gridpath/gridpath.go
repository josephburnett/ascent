// Package gridpath resolves a pane's descent path to its leaf grid. It is
// outside the wasm shim so it is unit tested: a stale-prefix slip sends you to
// the wrong grid.
package gridpath

// ResolveLeafGrid walks a descent path from rootGridID, following each well
// tile's child grid. Where lookup reports the grid uncached or the path id
// absent it returns the last good grid id, so a stale prefix resolves as deep
// as it can and never past a gap. The wasm caller reads the cache inside
// lookup and may kick a background fetch on a miss.
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
