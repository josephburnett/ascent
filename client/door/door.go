// Package door answers one question the bar keeps asking about a namespace
// level: what did the pane descend through? A place frame records the
// doorway's id, so the level's identity — its title, its rename target, its
// crumb glyph — is derived from the row. This is the one derivation;
// everything else reads it. Js-free and unit-tested.
package door

import (
	"google.golang.org/protobuf/proto"

	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"github.com/josephburnett/gridwell/api/rpc"
)

// EntrySeparator joins a menu row's name to one of its entries' names. See
// EntryName.
const EntrySeparator = " · "

// EntryName is what one of a row's declared menu entries is called, wherever
// it is shown: the + menu swatch, the drag ghost, the link tile a drag drops,
// and the crumb of the level it opens. An entry is a collection of the
// instance that declared it, and it stands alone on the menu with no plugin
// row above it, so it says whose it is — "hey · Feed".
//
// The rule is uniform: every entry of every row is named this way, with no
// privileged one. An entry that declares no label of its own wears the
// instance's name alone, which is the whole identity of a plugin with a
// single collection.
func EntryName(row, entry string) string {
	switch {
	case entry == "":
		return row
	case row == "":
		return entry
	}
	return row + EntrySeparator + entry
}

// EntryPlugin shapes one of a row's MenuEntries as a pseudo-row: the entry's
// grid as the root and EntryName plus the entry's glyph as the face, so every
// downstream flow (swatch, ghost, click-descend, drag-link, the bar's door
// identity) takes the ordinary row path. The pseudo-row is a doorway, not a
// declarer, so it carries none of the row's entries — they belong to the row,
// and composing them again would show every collection twice.
//
// The framing follows the grid, not the row: the row's root view belongs to
// the row's own grid, and the entry carries its own, so a collection reopens
// where it was left.
func EntryPlugin(pl *gridwellv1.PluginInfo, e *gridwellv1.MenuEntry) *gridwellv1.PluginInfo {
	pseudo := proto.Clone(pl).(*gridwellv1.PluginInfo)
	pseudo.RootGridId = e.GridId
	pseudo.RootViewCx, pseudo.RootViewCy, pseudo.RootViewZoom = e.ViewCx, e.ViewCy, e.ViewZoom
	pseudo.MenuEntries = nil
	pseudo.Label = EntryName(pl.Label, e.Label)
	if e.Glyph != "" {
		pseudo.Glyph = e.Glyph
	}
	return pseudo
}

// Kind says what the resolved door is, which decides renamability: a real
// well row takes the rename gesture; a declaration (a node's own home, a
// declared menu entry) is config-owned and read-only.
type Kind int

const (
	None Kind = iota
	// Well: a real tile row (the well actually descended through, or the
	// instance well naming the same place) — renamable where you stand.
	Well
	// Entry: a declared MenuEntry's pseudo swatch — one of a plugin's
	// collections, the home's trashcan.
	Entry
	// Root: a row's own swatch, for a row that is a place — a node's home,
	// a connection's far home. A plugin has none.
	Root
)

// WellInto finds the well tile whose child grid is anchor — the door a
// descent into anchor went through. The portal-ascent animation runs the
// same scan.
func WellInto(anchor string, tiles map[string]*gridwellv1.Tile) (*gridwellv1.Tile, bool) {
	for _, t := range tiles {
		if t.ChildGridId == anchor && rpc.IsWellKind(t.Kind) {
			return t, true
		}
	}
	return nil, false
}

// Find resolves the door into the level rooted at anchor, most-specific
// first:
//  1. the parent level's well whose child is anchor — the tile actually
//     descended through (grid-tile descents, adopted plugin wells);
//  2. a MenuEntry declaring anchor — a menu-swatch descent into one of a
//     plugin's collections, or the home's trashcan;
//  3. the row whose RootGridID is anchor — its own swatch (a node's home, a
//     connection's far home: its label is the row's, its framing the row's
//     view).
//
// A None result means the level has no derivable door (a workspace root,
// an uncached world) — callers keep their fallback.
func Find(anchor string, parentTiles map[string]*gridwellv1.Tile, plugins []*gridwellv1.PluginInfo) (*gridwellv1.Tile, Kind) {
	if anchor == "" {
		return nil, None
	}
	if t, ok := WellInto(anchor, parentTiles); ok {
		return t, Well
	}
	for _, pl := range plugins {
		for _, e := range pl.MenuEntries {
			if e.GridId == anchor {
				return rpc.PluginWellTile(EntryPlugin(pl, e)), Entry
			}
		}
	}
	for _, pl := range plugins {
		if pl.RootGridId == anchor {
			return rpc.PluginWellTile(pl), Root
		}
	}
	return nil, None
}

// EntryGlyph is the glyph a declared menu entry declares for gridID, or ""
// when no entry names it — the one override the grid itself cannot carry
// (the trash grid is an ordinary local grid; only the declaration knows its
// face).
func EntryGlyph(gridID string, plugins []*gridwellv1.PluginInfo) string {
	for i := range plugins {
		for _, e := range plugins[i].MenuEntries {
			if e.GridId == gridID && e.Glyph != "" {
				return e.Glyph
			}
		}
	}
	return ""
}

// RowGlyph is the face a + menu row wears — the one answer for its swatch,
// its drag ghost, and the crumb of the grid it roots, so an undeclared glyph
// cannot mean one thing in the menu and another in the bar. A row that
// declares a glyph keeps it everywhere; a row that declares none takes the
// grid face, because a plugin serves grids. A connection declares the globe
// where connection rows are minted (rpc.ConnectionRow), so nothing here
// switches on a kind.
func RowGlyph(pl *gridwellv1.PluginInfo) string {
	if pl.Glyph != "" {
		return pl.Glyph
	}
	return rpc.GlyphWell
}

// GlyphFor is the identity glyph for the plugin owning gridID, from
// DECLARATIONS only — no reader here knows a plugin's kind. Most specific
// first:
//
//  1. a root MenuEntry naming the grid: the trash grid is an ordinary local
//     grid, so only its entry knows its face;
//  2. a + menu row rooted exactly here: this grid IS that row, so it wears
//     the row's face (RowGlyph), the same square the menu draws. A
//     connection's root is the far node's home grid, which declares the far
//     node's own face; from here it is the connection, so the row wins;
//  3. the cached grid's own declared glyph (Grid.glyph, stamped by the
//     serving node from the owning plugin's Info), which answers for remote
//     grids a local plugin-list lookup cannot;
//  4. for content served by another node (node_ns set), the mount door's
//     declared glyph — the same face the tile you descended through wore.
//     The mount door is the connection row, uuid "<id>/<conn>"; the node's
//     own id prefixes it, so a prefix lookup would answer for home, not the
//     door. An unknown mount takes the globe, like every connection;
//  5. the plugin row's face from the handshake, looked up by the grid's
//     namespace, for a grid not cached yet.
//
// A cached grid that declares no glyph and came from this node is owned
// content: the well glyph. grid is nil when the client has not cached it.
// There is always an answer — a crumb with no face is a blank square.
func GlyphFor(gridID string, grid *gridwellv1.Grid, plugins []*gridwellv1.PluginInfo) string {
	if g := EntryGlyph(gridID, plugins); g != "" {
		return g
	}
	if pl, ok := ByRoot(gridID, plugins); ok {
		return RowGlyph(pl)
	}
	if grid != nil {
		if grid.Glyph != "" {
			return grid.Glyph
		}
		if grid.NodeNs != "" {
			if pl, ok := byUUID(grid.NodeNs, plugins); ok {
				return RowGlyph(pl)
			}
			return rpc.GlyphGlobe
		}
		return rpc.GlyphWell
	}
	if pl, ok := byUUID(rpc.UUIDOf(gridID), plugins); ok {
		return RowGlyph(pl)
	}
	return rpc.GlyphWell
}

// Place is one grid a menu row is a doorway onto: the row that names it —
// a pseudo-row for an entry — and the entry it was declared by, nil for the
// row's own grid.
type Place struct {
	Plugin *gridwellv1.PluginInfo
	Entry  *gridwellv1.MenuEntry
}

// PlacesOf enumerates the doorways one menu row declares, in declaration
// order: the row's own grid where it names one, then one per menu entry that
// names a grid. A plugin names no grid of its own, so its places are exactly
// its collections; a node's home and a connection's far home name one, since
// a node is a place.
//
// This is the one enumeration. The menu composes its swatches from it
// (client/palette), ByRoot looks a grid up in it, and the framing restore
// iterates it, so what a doorway is cannot be answered three ways.
func PlacesOf(pl *gridwellv1.PluginInfo) []Place {
	out := make([]Place, 0, 1+len(pl.MenuEntries))
	if pl.RootGridId != "" {
		out = append(out, Place{Plugin: pl})
	}
	for _, e := range pl.MenuEntries {
		if e.GridId == "" {
			continue
		}
		out = append(out, Place{Plugin: EntryPlugin(pl, e), Entry: e})
	}
	return out
}

// Places is PlacesOf over a whole menu, rows in order and each row's places
// directly after it.
func Places(plugins []*gridwellv1.PluginInfo) []Place {
	out := make([]Place, 0, len(plugins))
	for _, pl := range plugins {
		out = append(out, PlacesOf(pl)...)
	}
	return out
}

// ByRoot finds the doorway rooted exactly at gridID — the swatch this grid
// IS, a row's own or one of its declared entries'. Rooted, not by namespace:
// a connection row's uuid ("<id>/<conn>") is not a prefix of its root
// ("<id>/<conn>/<remote-home>/<n>").
func ByRoot(gridID string, plugins []*gridwellv1.PluginInfo) (*gridwellv1.PluginInfo, bool) {
	if gridID == "" {
		return nil, false
	}
	for _, p := range Places(plugins) {
		if p.Plugin.RootGridId == gridID {
			return p.Plugin, true
		}
	}
	return nil, false
}

// byUUID finds the plugin row with the given, possibly chain-qualified,
// namespace.
func byUUID(u string, plugins []*gridwellv1.PluginInfo) (*gridwellv1.PluginInfo, bool) {
	for _, pl := range plugins {
		if pl.Uuid == u {
			return pl, true
		}
	}
	return nil, false
}
