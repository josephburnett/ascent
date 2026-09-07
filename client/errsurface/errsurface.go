// Package errsurface owns the client's queue of user-visible failure notices.
// A failure that only reaches the console looks to the user like it just
// disappeared, so every layer that detects one reports here: wasm RPC
// dispatch, stream clients, the Electron host's error IPC event, and server
// health events. Only the render layer reads, and no other code holds or draws
// error state.
//
// The package is js-free and pure, so coalescing, ordering, capacity, expiry,
// strip geometry, and dismiss hit-testing are all table-testable without a
// browser. The wasm shell contributes pixels, timers, and event plumbing.
package errsurface

import (
	"fmt"
	"strings"
	"time"
)

// Severity classifies a notice for display. Error is an unexpected failure,
// something the user asked for that did not happen. Info is an expected
// reconciliation worth mentioning, such as a lost version race resolved by
// refetching. There is no debug tier, because the console is that.
type Severity int

const (
	Info Severity = iota
	Error
)

// Notice is one row on the surface.
type Notice struct {
	// ID is a monotonically assigned handle for dismissal, stable for the life
	// of the notice including across coalesced re-reports.
	ID int
	// Source is the stable key of the failure site, such as "rpc:MoveTile",
	// "events" or "electron:webview". One notice exists per source, so a retry
	// loop updates its row in place rather than scrolling the strip.
	Source string
	// Message is the most recent human-readable failure text for Source.
	Message  string
	Severity Severity
	// Count is how many times Source has reported since it was last dismissed
	// or resolved. It renders as a "×N" suffix when above 1.
	Count int
	// deadline is when this notice expires if its source stops reporting. It is
	// zero for sticky sources, which live until Dismiss or Resolve.
	deadline time.Time
}

// ExpireAfter is how long a non-sticky notice stays visible after its most
// recent report. A one-shot failure fades once it stops recurring, while a
// recurring one keeps refreshing its deadline through Report and stays up.
const ExpireAfter = 10 * time.Second

// Sticky reports whether source names an ongoing condition rather than a
// one-shot event. A sticky notice never expires, because it stands for a state
// reported once on the transition that would otherwise vanish while still
// true. Each sticky source has an exit: plugin health resolves on the recovery
// event, and the backend notice can only be dismissed, since nothing short of
// a restart recovers. This table is the one owner of the split, and report
// sites do not choose.
func Sticky(source string) bool {
	return source == "electron:backend" || strings.HasPrefix(source, "plugin:")
}

// maxNotices bounds the queue so an unattended failure loop cannot grow
// memory, dropping the oldest. It sits far above MaxRows because it is a
// safety valve rather than a display rule.
const maxNotices = 50

// Surface is the notice queue. The zero value is not ready, so use New. Like
// every other client-side store it is not safe for concurrent use, because the
// wasm client is single-threaded and its goroutines interleave without running
// in parallel.
type Surface struct {
	notices []Notice // index 0 is the newest
	nextID  int
}

func New() *Surface { return &Surface{nextID: 1} }

// Report adds a notice, or refreshes the existing one for the same source. The
// latest message and severity win, Count increments, and the row moves to the
// top keeping its ID. The caller passes now, which keeps the package clock-free
// and testable, and it restarts the expiry countdown, so a recurring failure
// stays visible while it recurs plus ExpireAfter of silence.
func (s *Surface) Report(sev Severity, source, message string, now time.Time) {
	deadline := now.Add(ExpireAfter)
	if Sticky(source) {
		deadline = time.Time{}
	}
	for i := range s.notices {
		if s.notices[i].Source == source {
			n := s.notices[i]
			n.Message = message
			n.Severity = sev
			n.Count++
			n.deadline = deadline
			s.notices = append(s.notices[:i], s.notices[i+1:]...)
			s.notices = append([]Notice{n}, s.notices...)
			return
		}
	}
	n := Notice{ID: s.nextID, Source: source, Message: message, Severity: sev, Count: 1, deadline: deadline}
	s.nextID++
	s.notices = append([]Notice{n}, s.notices...)
	if len(s.notices) > maxNotices {
		s.notices = s.notices[:maxNotices]
	}
}

// Expire drops every non-sticky notice whose deadline has passed and reports
// whether anything changed, so the caller knows to repaint. Expiry is an
// explicit mutation on the caller's clock tick, and reading never mutates.
func (s *Surface) Expire(now time.Time) bool {
	kept := s.notices[:0]
	for _, n := range s.notices {
		if n.deadline.IsZero() || n.deadline.After(now) {
			kept = append(kept, n)
		}
	}
	changed := len(kept) != len(s.notices)
	s.notices = kept
	return changed
}

// NextDeadline returns how long until the soonest pending expiry, and false
// when nothing expires. The wasm shell arms a single timer from it instead of
// polling. The duration can be zero or negative if a deadline has passed.
func (s *Surface) NextDeadline(now time.Time) (time.Duration, bool) {
	var soonest time.Time
	for _, n := range s.notices {
		if n.deadline.IsZero() {
			continue
		}
		if soonest.IsZero() || n.deadline.Before(soonest) {
			soonest = n.deadline
		}
	}
	if soonest.IsZero() {
		return 0, false
	}
	return soonest.Sub(now), true
}

// Notices returns a copy of the queue, newest first.
func (s *Surface) Notices() []Notice {
	out := make([]Notice, len(s.notices))
	copy(out, s.notices)
	return out
}

func (s *Surface) Len() int { return len(s.notices) }

// Dismiss removes the notice with the given ID. An unknown ID does nothing.
func (s *Surface) Dismiss(id int) {
	for i := range s.notices {
		if s.notices[i].ID == id {
			s.notices = append(s.notices[:i], s.notices[i+1:]...)
			return
		}
	}
}

// Resolve removes the notice for source, if any. It is how a cleared condition
// takes its notice down, such as the event stream resolving its own disconnect
// notice when it reconnects.
func (s *Surface) Resolve(source string) {
	for i := range s.notices {
		if s.notices[i].Source == source {
			s.notices = append(s.notices[:i], s.notices[i+1:]...)
			return
		}
	}
}

// ── strip geometry ───────────────────────────────────────────────────────────
//
// The strip is reserved layout rather than an overlay. The pane tree is laid
// out into the canvas height minus StripHeight, so a native WebContentsView,
// which tracks pane rects, can never cover the strip.

// RowH is the height of one notice row in CSS pixels.
const RowH = 24.0

// MaxRows caps how many notices are visible at once. The OverflowCount of the
// last visible row summarizes the rest.
const MaxRows = 3

// StripHeight is the canvas height to reserve for count pending notices.
func StripHeight(count int) float64 {
	if count <= 0 {
		return 0
	}
	if count > MaxRows {
		count = MaxRows
	}
	return float64(count) * RowH
}

// Row is one rendered line of the strip: the notice, its top edge in canvas
// coordinates, and how many further notices hide behind it. OverflowCount is
// non-zero only on the last visible row.
type Row struct {
	Notice        Notice
	Y             float64
	OverflowCount int
}

// Rows lays out the visible rows top down, newest first, from the strip's top
// edge in canvas coordinates. Render and hit-testing both read it, so they
// cannot disagree.
func Rows(notices []Notice, stripTop float64) []Row {
	n := len(notices)
	if n == 0 {
		return nil
	}
	vis := n
	if vis > MaxRows {
		vis = MaxRows
	}
	rows := make([]Row, vis)
	for i := 0; i < vis; i++ {
		rows[i] = Row{Notice: notices[i], Y: stripTop + float64(i)*RowH}
	}
	rows[vis-1].OverflowCount = n - vis
	return rows
}

// Label is the display text for a notice, either the message or the message
// with a "×N" suffix.
func Label(n Notice) string {
	if n.Count > 1 {
		return fmt.Sprintf("%s ×%d", n.Message, n.Count)
	}
	return n.Message
}

// DismissAt dismisses the notice whose row contains canvas y and reports
// whether anything changed. The caller has already established that x is on
// the canvas and y is at or below stripTop. The whole row is the dismiss
// target, with no separate close box.
func (s *Surface) DismissAt(y, stripTop float64) bool {
	rows := Rows(s.notices, stripTop)
	for _, r := range rows {
		if y >= r.Y && y < r.Y+RowH {
			s.Dismiss(r.Notice.ID)
			return true
		}
	}
	return false
}
