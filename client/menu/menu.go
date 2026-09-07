// Package menu owns the live state of the + creation menu: whether it is
// open, which pane it is open on, which palette item is hovered, and whether
// the doorway section is unfolded. The menu is open on at most one pane, the
// focused one, and every transition goes through a method here, so a new
// gesture-ending path cannot leave it stranded open on an unfocused pane.
//
// State is live state only. What survives a descent rides the place frame you
// left (pane.Frame.MenuOpen): record it with OpenOn at the descent and reopen
// with Open when the ascent lands on that frame.
package menu

// noHover is the hovered-item index when nothing is hovered.
const noHover = -1

// State is the + menu's live state. The zero value is invalid, because hover
// would read as item 0; construct with New.
type State struct {
	open     bool
	paneID   string
	hover    int
	expanded bool
}

// New returns a closed menu with no hovered item.
func New() State { return State{hover: noHover} }

// IsOpen reports whether the menu is open on any pane.
func (s *State) IsOpen() bool { return s.open }

// OpenOn reports whether the menu is open on paneID. Every per-pane site
// guards on it, and it is the snapshot a place frame records at a descent.
func (s *State) OpenOn(paneID string) bool { return s.open && s.paneID == paneID }

// PaneID is the pane the menu is open on. A closed menu reports "", so a
// stale id can never resolve to a pane.
func (s *State) PaneID() string {
	if !s.open {
		return ""
	}
	return s.paneID
}

// Hover returns the hovered palette-item index, or -1 when nothing is hovered.
func (s *State) Hover() int { return s.hover }

// PluginsExpanded reports whether the doorway section is unfolded. It is
// live state rather than a preference, so every opening starts collapsed.
func (s *State) PluginsExpanded() bool { return s.expanded }

// TogglePlugins flips the doorway section open or shut and returns the new
// state. The menu stays open on the same pane and the pane keeps its focus
// and selection. Hover is dropped, because the swatch list under the pointer
// has changed.
func (s *State) TogglePlugins() bool {
	s.expanded = !s.expanded
	s.hover = noHover
	return s.expanded
}

// Open opens the menu on paneID, resetting hover and the fold. Every opening
// starts collapsed.
func (s *State) Open(paneID string) {
	s.open = true
	s.paneID = paneID
	s.hover = noHover
	s.expanded = false
}

// Close clears the remembered pane, hover and fold, so nothing downstream
// can read a stale pane id off a closed menu.
func (s *State) Close() {
	s.open = false
	s.paneID = ""
	s.hover = noHover
	s.expanded = false
}

// Toggle is the + click: close if already open on paneID, otherwise open
// there. It returns the resulting open state.
func (s *State) Toggle(paneID string) bool {
	if s.OpenOn(paneID) {
		s.Close()
		return false
	}
	s.Open(paneID)
	return true
}

// SetHover sets the hovered palette-item index, -1 for none, and reports
// whether it changed, so the caller redraws only on a real change.
func (s *State) SetHover(i int) (changed bool) {
	if !s.open || s.hover == i {
		return false
	}
	s.hover = i
	return true
}

// SyncFocus closes a menu open on a pane that is no longer focused. Call it
// whenever focus moves, so no focus path has to close the menu itself.
func (s *State) SyncFocus(focusedPaneID string) {
	if s.open && s.paneID != focusedPaneID {
		s.Close()
	}
}

// TransferFocus reports whether focus changed and syncs the menu when it
// did. Every path that moves wasm focus calls it, so no caller has to
// remember the comparison, and calling it when focus has not moved is safe.
func (s *State) TransferFocus(prevID, newID string) bool {
	if prevID == newID {
		return false
	}
	s.SyncFocus(newID)
	return true
}
