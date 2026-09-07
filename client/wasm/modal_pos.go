//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/josephburnett/gridwell/client/panebox"
)

// centerCardOnActivePane puts a modal card over the active pane's center, so
// the dialog appears where you acted. The geometry is panebox.ModalCardPos'.
// Call it after the modal is visible, since the card must have layout to
// measure; margins are zeroed so the measured size is the placed size. With
// no laid-out focused pane the backdrop's flex centering stays.
func (a *App) centerCardOnActivePane(card js.Value) {
	_, r, ok := a.focusedPaneRect()
	if !ok {
		return
	}
	w := card.Get("offsetWidth").Float()
	h := card.Get("offsetHeight").Float()
	win := js.Global()
	x, y := panebox.ModalCardPos(r, w, h,
		win.Get("innerWidth").Float(), win.Get("innerHeight").Float())
	st := card.Get("style")
	st.Set("position", "fixed")
	st.Set("margin", "0")
	st.Set("left", fmt.Sprintf("%.0fpx", x))
	st.Set("top", fmt.Sprintf("%.0fpx", y))
}
