// Package barslot owns what the bottom bar's circle slot is for the focused
// pane. One verdict names both the affordance drawn and the action a click
// runs, so the button cannot promise one thing and do another. It carries no
// js/wasm build tag, so every arm is unit tested headlessly and the shim keeps
// the input gathering, the pixels, and the effect dispatch.
package barslot

// Mode is what the slot is for a given pane state, naming both a glyph and an
// action. ModeURLGoLive and ModeShellRefresh draw the same refresh glyph but
// stay separate modes, because one click places a url view and the other
// attaches a PTY.
type Mode int

const (
	// ModeNothing leaves the slot empty and a click on it does nothing. It
	// covers a markdown descent, whose slot holds the DOM text-mode toggle at
	// the same center, a live shell descent, and a frozen shell whose tmux
	// session is gone.
	ModeNothing Mode = iota
	// ModeURLBack is a live url descent; the click runs history.back().
	ModeURLBack
	// ModeURLGoLive is a frozen url descent on a host that can place a live
	// view; the click opens the url stream.
	ModeURLGoLive
	// ModeURLOpenTab is a frozen url descent on a host that cannot go live. The
	// click opens the address in a new browser tab and the tile stays frozen.
	ModeURLOpenTab
	// ModeShellRefresh is a frozen shell descent whose refresh button shows. The
	// click creates a tmux session for a never-opened tile, or attaches to the
	// existing one.
	ModeShellRefresh
	// ModePlus is a pane on a grid; the slot is the + menu toggle. The drawer
	// swaps in the trashcan while a tile drag is in flight.
	ModePlus
)

// Input is the world state the slot's mode reads. The caller resolves every
// field; nothing here is re-derived.
type Input struct {
	// Descent is whether the pane is inside a tile (pane.ContentID() != "").
	Descent bool
	// URLDescent is whether that tile presents as web content, which is a url
	// tile or a serves_page tile (rpc.WebContent).
	URLDescent   bool
	ShellDescent bool
	// URLLive is whether a native url view is placed on this pane.
	URLLive bool
	// ShellLive is whether a PTY stream is attached to this pane.
	ShellLive bool
	// CanLiveURL is caps.LiveURL, whether this host can place a live url view.
	CanLiveURL bool
	// ShellRefreshVisible is shellconn.DecideShellRefreshVisible's Show for the
	// descended shell tile. Only the frozen-shell arm reads it, and the caller
	// must resolve it lazily because resolving it kicks a liveness probe.
	ShellRefreshVisible bool
}

// Decide maps a pane's state to its slot mode. URLDescent is tested before
// ShellDescent: the two cannot both be true, since a shell tile is not web
// content, but the priority is fixed here rather than in each caller's arm
// order.
func Decide(in Input) Mode {
	if !in.Descent {
		return ModePlus
	}
	switch {
	case in.URLDescent:
		switch {
		case in.URLLive:
			return ModeURLBack
		case in.CanLiveURL:
			return ModeURLGoLive
		default:
			return ModeURLOpenTab
		}
	case in.ShellDescent:
		if !in.ShellLive && in.ShellRefreshVisible {
			return ModeShellRefresh
		}
	}
	return ModeNothing
}
