//go:build js && wasm

package main

import (
	"context"
	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"strings"
	"syscall/js"

	"github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/client/door"
	"github.com/josephburnett/gridwell/client/pane"
)

// Naming: name the room you are in. The focused pane's name renders as the
// bottom bar's centered title, in a band that is reserved layout below every
// pane, so the label and the inline rename input work identically over
// canvas, shells and live url panes. An unchanged value never writes, because
// reading never mutates.

// renameTarget returns the tile the focused pane's rename input edits, or
// false when nothing here is renamable. A url or shell descent edits that
// tile; a text tile's name derives from its first line, which the server
// refuses to override, and an ephemeral tile dies on ascent, so naming one is
// a lie. Inside a well's grid it is the containing well, since renaming the
// room names its door. A declared doorway is config-owned.
func (a *App) renameTarget(p *pane.Pane) (*gridwellv1.Tile, bool) {
	if p == nil {
		return nil, false
	}
	if p.ContentID() != "" {
		t, ok := a.descendedTile(p)
		if !ok || t.Kind == rpc.KindText || a.possiblyEphemeral(p, t) {
			return nil, false
		}
		return t, true
	}
	if len(p.Path()) == 0 {
		// At a namespace level the well descended through is a real row, so
		// renaming the room names its door. Declarations stay unrenamable.
		if t, kind := a.doorFind(p); kind == door.Well {
			return t, true
		}
		return nil, false
	}
	parentGridID := a.gridIDForPathFrom(p.Anchor(), p.Path()[:len(p.Path())-1])
	g, ok := a.c.Grid(parentGridID)
	if !ok {
		return nil, false
	}
	t, ok := g.Tiles[p.Path()[len(p.Path())-1]]
	if !ok || !rpc.IsWellKind(t.Kind) {
		return nil, false
	}
	return t, true
}

// bubbleDecorate applies pane-state markers to the title text. bubbleLabel is
// what the bar title shows and whether it is user-editable: the renameTarget's
// name, or else a read-only context label. Everything that shows the name
// renders bubbleLabel's output.
func (a *App) bubbleDecorate(p *pane.Pane, label string) string {
	if a.tree.Zoomed == p.ID {
		return "⛶ " + label
	}
	return label
}

func (a *App) bubbleLabel(p *pane.Pane) (label string, editable, muted bool) {
	if t, ok := a.renameTarget(p); ok {
		if t.AltText == "" {
			return "unnamed", true, true
		}
		return t.AltText, true, false
	}
	if p.ContentID() != "" {
		if t, ok := a.descendedTile(p); ok {
			if a.certainlyEphemeral(p, t) {
				return "ephemeral", false, true
			}
			if t.AltText != "" {
				return t.AltText, false, false // derived, read-only
			}
		}
		return "unnamed", false, true
	}
	// At a namespace level, the door's declared label. A renamable door was
	// already answered by the renameTarget arm above.
	if len(p.Path()) == 0 {
		if t, kind := a.doorFind(p); kind != door.None {
			if t.AltText == "" {
				return "unnamed", false, true
			}
			return t.AltText, false, true
		}
	}
	// An uncached parent: the plugin's config-owned label.
	want := uuidOf(a.gridIDForPane(p))
	for _, pl := range a.allPlugins() {
		if pl.Uuid == want && pl.Label != "" {
			return pl.Label, false, true
		}
	}
	return "unnamed", false, true
}

// doorFind resolves the tile the pane's current level was entered through,
// assembling client/door's inputs from the caches.
func (a *App) doorFind(p *pane.Pane) (*gridwellv1.Tile, door.Kind) {
	var parent map[string]*gridwellv1.Tile
	if p.Depth() > 1 {
		anchor, path := p.AnchorPathAt(p.Depth() - 2)
		if gid := a.gridIDForPathFrom(anchor, path); gid != "" {
			if g, ok := a.c.Grid(gid); ok {
				parent = g.Tiles
			} else {
				a.fetchGrid(gid)
			}
		}
	}
	return door.Find(p.Anchor(), parent, a.allPlugins())
}

// togglePaneZoom is the left-click on the bar's centered title.
func (a *App) togglePaneZoom() {
	p := a.tree.FocusedPane()
	if p == nil {
		return
	}
	a.menu.Close()
	a.tree.ToggleZoom(p.ID)
	a.draw()
	a.scheduleURLUpdate()
}

// openNameInputAt spawns the one inline rename input, the same DOM shape and
// keys for every rename surface. onCommit receives the trimmed value on Enter
// or on blur, because a phone keyboard's done key blurs and a typed name must
// not be silently discarded. An unchanged value never writes, because reading
// never mutates.
func (a *App) openNameInputAt(value string, width float64, position func(st js.Value), onCommit func(string)) {
	doc := js.Global().Get("document")
	in := doc.Call("createElement", "input")
	in.Set("id", "gw-rename-input")
	in.Set("value", value)
	st := in.Get("style")
	st.Set("position", "absolute")
	st.Set("zIndex", "8")
	st.Set("background", colorMenuBg)
	st.Set("border", "1px solid "+colorFocusBorder)
	st.Set("borderRadius", "10px")
	st.Set("padding", "1px 10px")
	st.Set("font", "12px sans-serif")
	st.Set("color", colorMenuItemHi)
	st.Set("outline", "none")
	st.Set("width", pxf(width))
	position(st)
	a.overlays.renameEditing = true

	orig := strings.TrimSpace(value)
	closed := false
	var keyCb, blurCb js.Func
	closeInput := func(commit bool) {
		if closed {
			return
		}
		closed = true
		val := strings.TrimSpace(in.Get("value").String())
		in.Call("remove")
		keyCb.Release()
		blurCb.Release()
		a.overlays.renameEditing = false
		if commit && val != orig {
			onCommit(val)
		}
		a.draw()
	}
	keyCb = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ev := args[0]
		ev.Call("stopPropagation")
		switch ev.Get("key").String() {
		case "Enter":
			closeInput(true)
		case "Escape":
			closeInput(false)
		}
		return nil
	})
	blurCb = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		closeInput(true) // blur commits; see the doc comment
		return nil
	})
	in.Call("addEventListener", "keydown", keyCb)
	in.Call("addEventListener", "blur", blurCb)
	doc.Get("body").Call("appendChild", in)
	in.Call("focus")
	in.Call("select")
	a.draw() // hides the title while editing
}

// commitRename posts the user-owned name and patches the cache so the title
// reflects it immediately. The TileChanged event confirms.
func (a *App) commitRename(tileID, alt string) {
	a.commitRenameRetained(tileID, alt, func(t *gridwellv1.Tile) {
		a.c.UpdateTile(t.GridId, t)
	})
}

// commitRenameRetained is the one rename commit, running `apply` on success.
// The input element is gone by the time an RPC fails, so the closure parked
// in the outbox is the only copy of what the user typed.
func (a *App) commitRenameRetained(tileID, alt string, apply func(*gridwellv1.Tile)) {
	var tile *gridwellv1.Tile
	a.post(write{
		label: "Rename", gid: a.gridIDOfTile(tileID), id: tileID,
		source: "rename", failText: "rename",
		call: func(ctx context.Context) error {
			var err error
			tile, err = a.postRename(ctx, tileID, alt)
			return err
		},
		then: func() {
			if tile != nil {
				apply(tile)
			}
			a.draw()
		},
	})
}

// postRename is the one rename door. A name the user types is a content
// edit, so it claims a version and bumps one. A conflict surfaces rather than
// re-claiming: captures do not bump the row, so a conflict here is a genuine
// concurrent edit.
func (a *App) postRename(ctx context.Context, tileID, alt string) (*gridwellv1.Tile, error) {
	version := int64(0)
	if t := a.cachedTileByID(tileID); t != nil {
		version = t.Version
	}
	return a.cl.RenameTile(ctx, tileID, version, alt)
}

// gridIDOfTile is which grid's cache reconciles if a write is refused, for a
// call site that holds only an id. "" makes the dispatcher's refetch a no-op,
// since a grid this client never loaded has nothing to reconcile.
func (a *App) gridIDOfTile(tileID string) string {
	if t := a.cachedTileByID(tileID); t != nil {
		return t.GridId
	}
	return ""
}
