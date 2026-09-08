package palette

// What a bare click on a palette swatch means. The popover floats over a live
// canvas, so a click the palette does not claim reaches the gesture behind it
// and acts on whatever tile sits there. Every swatch therefore names a
// behavior, and a table gives a new kind the ClickNothing default.

type ClickTarget int

const (
	// ClickNothing leaves the menu open and the pane untouched.
	ClickNothing ClickTarget = iota
	// ClickEnter descends into the grid a doorway swatch names.
	ClickEnter
	// ClickHere is the bar's promote crumb: the pane already shows that
	// visit, so a click does nothing.
	ClickHere
	// ClickVisit opens the ephemeral visit without placing a tile.
	ClickVisit
)

// Swatch carries no coordinate, because a click has no destination.
type Swatch struct {
	// IsPlugin marks a plugin, connection or declared-root row.
	IsPlugin bool
	// Promote marks the bar's current-visit crumb, dragged as a template.
	Promote bool
	// Visits marks a primitive whose table row declares a click behavior.
	Visits bool
}

// ClickOn asks identity before kind, because a row can carry more than one
// flag: a plugin row's primitive fields are zero and the promote crumb is
// spelled as a url template.
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
