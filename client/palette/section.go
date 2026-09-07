package palette

// What the + menu shows. The primitives are the pane's own tiles and are
// always there. The swatches above them are an open set, since a node
// declares as many doorways as it likes, so that section folds behind a
// chevron. The fold lasts one menu opening and every opening starts
// collapsed; client/menu owns the flag and clears it on Open. Layout is the
// geometry of the rows this names.

// Section counts the doorway swatches the pane's node declares and the
// primitives its grid takes, which is zero on a read-only grid.
type Section struct {
	Plugins    int
	Primitives int
	Expanded   bool
}

// Chevron is the disclosure control's face.
type Chevron int

const (
	// ChevronNone means the popover carries no toggle.
	ChevronNone Chevron = iota
	// ChevronUp is collapsed: click to open the section above.
	ChevronUp
	// ChevronDown is expanded: click to fold it back up.
	ChevronDown
)

// String names the chevron so a spec can pin the face without reading
// pixels.
func (c Chevron) String() string {
	switch c {
	case ChevronUp:
		return "up"
	case ChevronDown:
		return "down"
	}
	return "none"
}

// Shown is what one menu opening displays.
type Shown struct {
	Plugins bool
	Toggle  bool
	Chevron Chevron
}

// Show decides what one menu opening displays. Two states have no toggle,
// because in both there is nothing to fold: a node that declares no doorways
// has no section, and a read-only grid offers no primitives, so folding the
// section would leave the popover empty.
func Show(s Section) Shown {
	if s.Plugins <= 0 {
		return Shown{}
	}
	if s.Primitives <= 0 {
		return Shown{Plugins: true}
	}
	if s.Expanded {
		return Shown{Plugins: true, Toggle: true, Chevron: ChevronDown}
	}
	return Shown{Toggle: true, Chevron: ChevronUp}
}
