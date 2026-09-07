package palette

// WHICH SWATCHES THE + MENU'S PLUGIN SECTION SHOWS. One per declared doorway,
// and nothing else. A node is a place — its home, and a connection's far home
// — so a row that names a grid of its own is a swatch. A plugin is not a
// place: it contributes collections, each declared as a menu entry, and each
// entry is a swatch. A plugin therefore has no row of its own on the menu and
// no privileged collection, so a plugin with three collections shows three
// swatches and a plugin with none shows nothing at all.
//
// A row that is broken or still waiting is a swatch too. That is the same
// rule, not an exception to it: pluginhealth.NoDoor is exactly "answered, and
// it is not a place", and every other status is a row the menu has something
// to say about — where you can go, or why you cannot.
//
// This is the composition decision, kept out of the shim so it is
// table-tested. The geometry of the rows it produces is Layout, what a fold
// state shows is Show, and the drawing is the renderer.

import (
	"github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/client/door"
	"github.com/josephburnett/gridwell/client/pluginhealth"
)

// Doorway is one swatch of the plugin section: the pseudo-row every
// downstream flow reads — face, ghost, click-descend, drag-link, health — and
// the menu entry it came from, nil when the swatch is a row's own place.
type Doorway struct {
	Plugin rpc.PluginInfo
	Entry  *rpc.MenuEntry
}

// Doorways composes the section from a handshake's menu rows (rpc.MenuRows:
// the node's plugins, home first, then its connections), in that order, each
// row's entries directly after it. An entry with no grid is not a doorway and
// is skipped: the far side declared a collection the node could not resolve.
func Doorways(rows []rpc.PluginInfo) []Doorway {
	out := make([]Doorway, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if pluginhealth.Classify(row) != pluginhealth.NoDoor {
			out = append(out, Doorway{Plugin: row})
		}
		for j := range row.MenuEntries {
			e := &row.MenuEntries[j]
			if e.GridID == "" {
				continue
			}
			out = append(out, Doorway{Plugin: door.EntryPlugin(row, *e), Entry: e})
		}
	}
	return out
}
