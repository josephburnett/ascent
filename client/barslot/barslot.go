// Package barslot owns what the bottom bar's circle slot is for the focused
// pane. One verdict names both the affordance drawn and the action a click
// runs, so the button cannot promise one thing and do another. It is js-free;
// the shim keeps the input gathering, the pixels and the effect dispatch.
package barslot

// Mode names both a glyph and an action. ModeURLGoLive and ModeShellRefresh
// draw the same refresh glyph but stay separate modes, one click placing a url
// view and the other attaching a PTY.
type Mode int

const (
	// ModeNothing covers a markdown descent, whose slot holds the DOM
	// text-mode toggle at the same center, a live shell descent, and a frozen
	// shell whose tmux session is gone.
	ModeNothing Mode = iota
	// ModeURLBack runs history.back().
	ModeURLBack
	// ModeURLGoLive opens the url stream.
	ModeURLGoLive
	// ModeURLOpenTab opens the address in a new browser tab; the tile stays
	// frozen.
	ModeURLOpenTab
	// ModeShellRefresh creates a tmux session for a never-opened tile, or
	// attaches to the existing one.
	ModeShellRefresh
	// ModePlus is the + menu toggle. The drawer swaps in the trashcan while a
	// tile drag is in flight.
	ModePlus
)

// Input is the world state the slot's mode reads. The caller resolves every
// field; nothing here is re-derived.
type Input struct {
	// Descent is pane.ContentID() != "".
	Descent bool
	// URLDescent is rpc.WebContent: a url tile or a serves_page tile.
	URLDescent   bool
	ShellDescent bool
	URLLive      bool
	ShellLive    bool
	// CanLiveURL is caps.LiveURL.
	CanLiveURL bool
	// ShellRefreshVisible is shellconn.DecideShellRefreshVisible's Show. Only
	// the frozen-shell arm reads it, and the caller must resolve it lazily
	// because resolving it kicks a liveness probe.
	ShellRefreshVisible bool
}

// Decide tests URLDescent before ShellDescent. The two cannot both be true, a
// shell tile not being web content, but the priority is fixed here rather than
// in each caller's arm order.
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
