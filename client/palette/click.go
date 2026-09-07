package palette

// What a bare click on a palette swatch means: press and release on the
// swatch with no drag between them. The popover floats over a live canvas, so
// a click the palette does not claim reaches the gesture behind it and acts
// on whatever tile sits at those coordinates. Every swatch therefore names a
// behavior, and a table gives a new kind the ClickNothing default.

// ClickTarget is the one behavior a bare click on a swatch runs.
type ClickTarget int

const (
	// ClickNothing leaves the menu open and the pane untouched.
	ClickNothing ClickTarget = iota
	// ClickEnter descends into the grid a doorway swatch names.
	ClickEnter
	// ClickHere is the bar's promote crumb, which stands for the visit the
	// pane already shows, so a click does nothing.
	ClickHere
	// ClickVisit opens the ephemeral visit a url or shell swatch declares,
	// without placing a tile.
	ClickVisit
)

// Swatch is what a bare click reads off one palette item. It carries no
// coordinate, because a click has no destination.
type Swatch struct {
	// IsPlugin marks a plugin, connection or declared-root row.
	IsPlugin bool
	// Promote marks the bar's current-visit crumb, dragged as a template.
	Promote bool
	// Visits marks a primitive whose table row declares a click behavior.
	Visits bool
}

// ClickOn maps a swatch to its click behavior. A row can carry more than one
// flag, so identity is asked before kind: a plugin row's primitive fields are
// zero, and the promote crumb is spelled as a url template.
func ClickOn(s Swatch) ClickTarget {
	switch {
	case s.IsPlugin:
		return ClickEnter
	case s.Promote:
		return ClickHere
	case s.Visits:
		return ClickVisit
	default:
		return ClickNothing
	}
}
