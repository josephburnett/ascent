package palette

// WHICH SWATCHES THE + MENU'S PLUGIN SECTION SHOWS. One per declared doorway,
// and nothing else. A node is a place — its home, and a connection's far home
// — so a row that names a grid of its own is a swatch. A plugin is not a
// place: it contributes collections, each declared as a menu entry, and each
// entry is a swatch. A plugin therefore has no row of its own on the menu and
// no privileged collection, so a plugin with three collections shows three
// swatches and a plugin with none shows nothing at all.
//
// A row with no doorway at all is a swatch only when the menu has something
// to say about it: it is broken, or it is still waiting on an answer. That is
// not an exception but the one thing beside the rule — a swatch is somewhere
// to go, or the reason there is nowhere.
//
// What a doorway IS, and what it is called, is client/door's (PlacesOf,
// EntryName). This decides which of them the menu shows, kept out of the shim
// so it is table-tested. The geometry of the rows it produces is Layout, what
// a fold state shows is Show, and the drawing is the renderer.

import (
	"github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/client/door"
	"github.com/josephburnett/gridwell/client/pluginhealth"
)

// Doorways composes the section from a handshake's menu rows (rpc.MenuRows:
// the node's plugins, home first, then its connections), in that order, each
// row's declared entries directly after it.
func Doorways(rows []rpc.PluginInfo) []door.Place {
	out := make([]door.Place, 0, len(rows))
	for i := range rows {
		row := rows[i]
		places := door.PlacesOf(row)
		out = append(out, places...)
		if len(places) == 0 && pluginhealth.Classify(row) != pluginhealth.NoDoor {
			out = append(out, door.Place{Plugin: row})
		}
	}
	return out
}
