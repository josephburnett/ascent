//go:build js && wasm

package main

import (
	"google.golang.org/protobuf/proto"

	"context"
	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"
	"syscall/js"

	"github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/client/pane"
)

// Content zoom: Ctrl/Cmd +/-/0 while descended into a text, shell or url tile
// scales the content. The zoom is per-tile framing, server-owned and never
// bumping version, restored on every descent.

const (
	contentZoomStep = 1.1
	contentZoomMin  = 0.5
	contentZoomMax  = 3.0
	// shellBaseFontPx is the terminal font at zoom 1.0.
	shellBaseFontPx = 13.0
)

func contentZoomOf(t *gridwellv1.Tile) float64 {
	if t != nil && t.ContentZoom > 0 {
		return t.ContentZoom
	}
	return 1.0
}

func clampContentZoom(z float64) float64 {
	if z < contentZoomMin {
		return contentZoomMin
	}
	if z > contentZoomMax {
		return contentZoomMax
	}
	return z
}

// textScaleFor is the render transform for a descended text pane. The
// painter, the wrap width and the textarea box all derive from it, so they
// cannot disagree about how big the text is.
func (a *App) textScaleFor(p *pane.Pane) float64 {
	if t, ok := a.descendedTile(p); ok {
		return textFixedScale * contentZoomOf(t)
	}
	return textFixedScale
}

// handleContentZoomKey consumes a Ctrl/Cmd +/-/0 chord for the focused
// descended pane. True means the caller stops.
func (a *App) handleContentZoomKey(ev js.Value) bool {
	if !(ev.Get("ctrlKey").Bool() || ev.Get("metaKey").Bool()) {
		return false
	}
	next := contentZoomNext(ev.Get("key").String())
	if next == nil {
		return false
	}
	p := a.tree.FocusedPane()
	if p == nil || p.ContentID() == "" {
		return false
	}
	// The content-descent kind set has one owner, rpc.IsContentDescentKind.
	t, ok := a.descendedTile(p)
	if !ok || !rpc.IsContentDescentKind(t.Kind) {
		return false
	}
	ev.Call("preventDefault")
	a.applyContentZoom(p, t, next(contentZoomOf(t)))
	return true
}

// contentZoomNext maps a zoom-chord key to its step function, nil for a key
// outside the chord. One mapping for the canvas keydown and the live-view
// forward, so they cannot step differently.
func contentZoomNext(key string) func(cur float64) float64 {
	switch key {
	case "+", "=":
		return func(c float64) float64 { return clampContentZoom(c * contentZoomStep) }
	case "-":
		return func(c float64) float64 { return clampContentZoom(c / contentZoomStep) }
	case "0":
		return func(float64) float64 { return 1.0 }
	}
	return nil
}

// contentZoomKeyFromView applies a zoom chord forwarded from a live URL view.
// The view owns OS keyboard focus, so the window-level keydown never fires
// and main relays the chord keyed by pane.
func (a *App) contentZoomKeyFromView(paneID, key string) {
	next := contentZoomNext(key)
	if next == nil {
		return
	}
	p := a.tree.FindPane(paneID)
	if p == nil || p.ContentID() == "" {
		return
	}
	t, ok := a.descendedTile(p)
	if !ok || !rpc.IsContentDescentKind(t.Kind) {
		return
	}
	a.applyContentZoom(p, t, next(contentZoomOf(t)))
}

// applyContentZoom updates the cache, pokes the live surface for the kinds
// that hold native state, and persists, the last only for a descent that
// outlives the pane leaving it.
func (a *App) applyContentZoom(p *pane.Pane, t *gridwellv1.Tile, z float64) {
	if rpc.PageContent(t) {
		// A serves_page descent has no persisted content_zoom, because the
		// owning plugin stores no url state and a client-only zoom would
		// break the no-client-state rule.
		return
	}
	nt := proto.CloneOf(t)
	nt.ContentZoom = z
	a.c.UpdateTile(nt.GridId, nt)
	switch t.Kind {
	case rpc.KindText:
		// Keep the pane's live scale, which the scroll math divides by, in
		// step with what the next draw reads.
		p.TextZoom = textFixedScale * z
	case rpc.KindShell:
		a.applyShellZoom(p.ID, z)
	case rpc.KindURL:
		a.bridgeSetZoom(p.ID, z)
	}
	a.refreshFileOverlay() // textarea font tracks the scale in text mode
	a.draw()
	// The zoom is live for the session either way; only the write is
	// conditional. An ephemeral visit's row dies on ascent, so persisting its
	// zoom would mark a row the user never asked to keep.
	if a.possiblyEphemeral(p, t) {
		return
	}
	// Through the framing dispatcher like every other framing write, because
	// a fire-and-forget call would reconcile no verdict and a transport
	// failure would leave the zoom client-only. There is no beacon form,
	// content zoom being the one framing write without one, so a quit inside
	// its settle window still loses it.
	tileID := t.Id
	a.postFramingPersist("SetContentZoom", nt.GridId, tileID,
		func(ctx context.Context) error {
			_, err := a.cl.SetContentZoom(ctx, tileID, z)
			return err
		}, nil)
}

// applyShellZoom sets the live terminal's font. The per-draw overlay sync
// re-fits the cell grid, which resizes the PTY to match.
func (a *App) applyShellZoom(paneID string, z float64) {
	if conn := a.shellConnFor(paneID); conn != nil {
		conn.term.Get("options").Set("fontSize", int(shellBaseFontPx*z+0.5))
	}
}
