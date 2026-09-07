package palette

// Which swatches the + menu's top section shows: one per declared doorway. A
// node is a place, so its home and a connection's far home each get a swatch.
// A plugin is not a place; it contributes collections, one menu entry each,
// so a plugin with three collections shows three swatches and a plugin with
// none shows nothing.
//
// A row that declares no doorway is a swatch only when the menu has something
// to say about it: it is broken, or still waiting on an answer.
//
// client/door decides what a doorway is and what it is called (PlacesOf,
// EntryName). Layout is the geometry and Show is what a fold state shows.

import (
	"github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/client/door"
	"github.com/josephburnett/gridwell/client/pluginhealth"
)

// Doorways composes the section from a handshake's menu rows, in the order
// rpc.MenuRows gives them, each row's declared entries directly after it.
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
