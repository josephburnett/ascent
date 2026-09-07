// Package panestate holds the per-pane session-local client state that is not
// the pane's place, which today is the selection. None of it is persisted,
// because the server owns durable state. It is a package of its own, and
// js-free, so the data is unit-tested as plain Go instead of living as parallel
// maps on the wasm App struct.
package panestate

// State is the plain-data per-pane client state.
type State struct {
	// Selected is the selected tile id, empty when nothing is selected.
	Selected string
	// There is deliberately no per-pane unsaved-edit mark. That fact is
	// tile-scoped, see cache.DirtyContent; a pane-scoped copy is reset by the
	// pane's next descent and strands the edit it describes.
}

// New returns an empty pane state.
func New() State { return State{} }
