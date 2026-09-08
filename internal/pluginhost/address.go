package pluginhost

// The derived address: how the node names a plugin entry, minted row or not. A
// key-form segment (rpc.KeyTileID) carries a payload only this package reads:
//
//	grid: the context key                    "~" + b64("/home/joe")
//	tile: the context key, NUL, the entry key "~" + b64("/home" NUL "/home/joe")
//
// The context half makes a tile answerable on its own, since plugin.v1 has no
// verb describing one entry. A key→context index would be a second copy of the
// plugin's structure written on every listing, so instead GetTile on an
// untouched entry is one List of the context the address names.

import (
	"strings"

	"github.com/josephburnett/gridwell/api/rpc"
)

// addrSep separates the context half of a tile address from the entry key.
const addrSep = "\x00"

// gridAddr renders a context key as a grid segment.
func gridAddr(context string) string { return rpc.KeyTileID(context) }

// tileAddr renders an entry as a tile segment: the context that lists it and
// its key. It is the entry's one public id; the row a first durable fact mints
// is bookkeeping, resolved on the way in (Adapter.resolveTile) and never handed
// out. A mint that renamed the entry would take the id out from under whoever
// was standing on it, so a URL restore or a descended pane would lose its
// target the moment a scroll minted a row.
func tileAddr(context, key string) string { return rpc.KeyTileID(context + addrSep + key) }

// splitAddr decodes a key-form segment. isTile is true when the payload
// carries an entry key rather than a bare context.
func splitAddr(seg string) (context, key string, isTile, ok bool) {
	payload, ok := rpc.TileKey(seg)
	if !ok {
		return "", "", false, false
	}
	context, key, isTile = strings.Cut(payload, addrSep)
	return context, key, isTile, true
}
