//go:build js && wasm

package main

// The menu context: the + menu belongs to the node a pane is inside, so
// descending into a node puts you there. A context is one node's plugin list
// plus its shells flag, keyed by the pane's grid's node_ns ("" is this node,
// the boot handshake). Remote contexts are fetched through the routed
// Handshake, with ids re-qualified for this receiver, and cached for the
// session; the source cache makes the fetch answer even while the mount is
// dark.

import (
	"context"
	gridwellv1 "github.com/josephburnett/gridwell/api/gen/gridwell/v1"

	"github.com/josephburnett/gridwell/api/rpc"
	"github.com/josephburnett/gridwell/client/pane"
)

// menuContext is one node's menu: its plugins and its shell policy.
type menuContext struct {
	plugins        []*gridwellv1.PluginInfo
	shellsDisabled bool
	// fetched marks a completed load. What keeps concurrent opens to one
	// read is NOT here: it is a.fetch.menuFetch, the client's one claim
	// mechanism, so this read is bounded and cancellable like every other. A
	// bare flag here was neither, and a Handshake the network swallowed held
	// it for the life of the page — the remote pane's menu then had no
	// plugin section at all, ever, and nothing was ever said about it.
	fetched bool
}

// paneNodeNS returns the namespace chain of the node serving pane p's current
// grid — the menu-context key. "" for the local node and for an uncached
// grid: until the grid loads nothing about the pane is renderable, the
// primitives are already hidden by the writable gate, and the local list is
// the least-wrong face.
func (a *App) paneNodeNS(p *pane.Pane) string {
	return a.gridNodeNS(a.gridIDForPane(p))
}

// gridNodeNS is paneNodeNS by grid id: the node serving that grid, read off
// the grid's own stamp. A drop resolves its destination grid rather than a
// pane's leaf grid — the two differ when the cursor promoted into an open
// well — so the same-node gate reads the grid it is actually landing in.
func (a *App) gridNodeNS(gridID string) string {
	if g, ok := a.c.Grid(gridID); ok {
		return g.Meta.NodeNs
	}
	return ""
}

// menuCtx returns the context for pane p, kicking a background fetch for
// a remote context not yet loaded (the menu redraws when it lands). The
// "" context is the boot handshake — always present, never fetched here.
func (a *App) menuCtx(p *pane.Pane) *menuContext {
	ns := a.paneNodeNS(p)
	if ns == "" {
		return &menuContext{plugins: a.plugins, shellsDisabled: !a.caps.Shells, fetched: true}
	}
	mc, ok := a.views.menuCtxs[ns]
	if !ok {
		mc = &menuContext{}
		a.views.menuCtxs[ns] = mc
	}
	if !mc.fetched {
		if ctx, done, ok := a.fetch.menuFetch.Begin(ns); ok {
			go a.fetchMenuCtx(ctx, done, ns)
		}
	}
	return mc
}

// fetchMenuCtx loads one remote node's menu through the routed Handshake, on
// the claim menuCtx opened for it. A failure leaves the context unfetched and
// surfaces, and the claim ends with the read — bounded, so a read the network
// swallows gives up and says so, and the next draw of the menu asks again.
// Nothing else retries it: an unfetched context is asked for by every draw of
// the open menu, which is the retry.
func (a *App) fetchMenuCtx(ctx context.Context, done func(), ns string) {
	defer done()
	lp, err := a.cl.HandshakeNS(ctx, ns)
	if err != nil {
		// reportErr schedules a frame, so the failure is both said and
		// re-asked: the next draw of the menu finds no claim and no context
		// and starts a fresh read over whatever link there now is.
		a.surfaceRPCError("Handshake", err)
		return
	}
	mc := a.views.menuCtxs[ns]
	mc.plugins = rpc.MenuRows(lp)
	mc.shellsDisabled = lp.ShellsDisabled
	mc.fetched = true
	a.draw()
}
