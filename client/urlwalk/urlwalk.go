// Package urlwalk holds the boot-time descent walk: it resolves a URL's
// tile-id list against the user's grids into a descent path plus an optional
// trailing content tile. The state machine lives here rather than in the wasm
// shim so `go test` covers it, because a misstep lands the user in the wrong
// grid.
package urlwalk

// Tile is the minimum a walk step needs to know about a tile: whether it
// descends into a child grid, whether it is a content leaf, and which grid a
// well points at. The caller classifies kinds with rpc.IsWellKind and
// rpc.IsContentDescentKind, which keeps this package free of wire types.
type Tile struct {
	ChildGridID string
	IsWell      bool
	IsContent   bool
}

// GridLookup returns the tiles of grid gid, fetching and caching it if needed.
// It returns false when the grid cannot be loaded, and the walk then stops
// where it is, because a failed fetch never invents a path.
type GridLookup func(gid string) (tiles map[string]Tile, ok bool)

// Walk resolves tileIDs against the grids reachable from rootGridID and returns
// the descent path of well ids in order, plus the trailing file-tile id, which
// is "" when the leaf is a grid.
//
// The rules are loose on input, so a bookmarked URL degrades gracefully as the
// canvas changes underneath it:
//   - An id missing from the current grid is skipped, and the walk stays in
//     the same grid and tries the next id.
//   - A well id is appended to the path and the walk descends into its child
//     grid.
//   - A content tile is accepted only as the last id, and one mid-path is
//     skipped.
//   - A grid that fails to load ends the walk with what resolved so far.
func Walk(rootGridID string, tileIDs []string, lookup GridLookup) (path []string, fileTileID string) {
	gid := rootGridID
	path = []string{}
	for i, id := range tileIDs {
		isLast := i == len(tileIDs)-1
		tiles, ok := lookup(gid)
		if !ok {
			break
		}
		t, ok := tiles[id]
		if !ok {
			// The id is stale or bogus, so keep the current grid.
			continue
		}
		switch {
		case t.IsWell:
			path = append(path, id)
			gid = t.ChildGridID
		case t.IsContent:
			if !isLast {
				// A content tile mid-path is nonsense.
				continue
			}
			fileTileID = id
		}
	}
	return path, fileTileID
}
