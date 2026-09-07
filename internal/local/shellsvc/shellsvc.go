// Package shellsvc owns the live shell PTY mechanics: the tmux-backed session
// lifecycle. It belongs to the namespace that owns the shell tiles, so live
// bytes cross the namespace interface through OpenShell like everything else,
// the server stays a pure bridge, and a shell in a remote namespace streams
// over the same path.
//
// A gridwell-private tmux server backs every shell tile, so a shell and its
// scrollback survive ascents and restarts. Tile ids here are namespace-local;
// tmux.SessionName maps one to a tmux session name.
package shellsvc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/josephburnett/gridwell/internal/local/shelldriver"
	"github.com/josephburnett/gridwell/internal/local/tmux"
)

// Sizing defaults and clamps for the PTY, exported so the OpenShell binder
// and the resize path agree on one set of bounds.
const (
	MinCols     = 20
	MinRows     = 5
	DefaultCols = 80
	DefaultRows = 24
)

// ErrSessionGone is returned by Acquire when the tile has been snapshotted but
// its tmux session is no longer alive. The shell is gone and only the JPEG
// remains, so the caller signals the client to hide the refresh button rather
// than fabricate a fresh session behind the snapshot.
var ErrSessionGone = errors.New("shell session no longer alive")

// Session is the per-tile PTY handle. Output is a channel so a takeover or
// detach returns at once, leaving no goroutine orphaned on a PTY syscall.
type Session interface {
	Output() <-chan []byte
	Write(p []byte) (int, error)
	Resize(cols, rows uint16) error
	Done() <-chan struct{}
	Close() error
}

// Streamer is the tmux and PTY backend. It is stubbed in tests so the manager
// can run without spawning a real pair.
type Streamer interface {
	OpenSession(tileID string, mode tmux.Mode, cols, rows uint16) (Session, error)
	HasSession(tileID string) (bool, error)
	Kill(tileID string) error
	ListLiveTileIDs() ([]string, error)
	PaneCommand(tileID string) (string, error)
}

// NewLive is the production Streamer. It composes the tmux argv through the
// controller and execs it through shelldriver, so a detach kills only the
// client and leaves the underlying tmux server running.
func NewLive(ctrl *tmux.Controller) Streamer { return &liveStreamer{ctrl: ctrl} }

type liveStreamer struct{ ctrl *tmux.Controller }

func (l *liveStreamer) OpenSession(tileID string, mode tmux.Mode, cols, rows uint16) (Session, error) {
	argv := l.ctrl.Args(tileID, mode, cols, rows, "")
	if len(argv) == 0 {
		return nil, fmt.Errorf("shellsvc: empty tmux argv for tile %s mode %v", tileID, mode)
	}
	// ctrl.Env carries the shadow-launcher PATH; see tmux.Controller.Env.
	return shelldriver.Start(shelldriver.Config{Cols: cols, Rows: rows, BashPath: argv[0], Args: argv[1:], Env: l.ctrl.Env()})
}

func (l *liveStreamer) HasSession(tileID string) (bool, error)    { return l.ctrl.HasSession(tileID) }
func (l *liveStreamer) Kill(tileID string) error                  { return l.ctrl.KillSession(tileID) }
func (l *liveStreamer) ListLiveTileIDs() ([]string, error)        { return l.ctrl.ListSessions() }
func (l *liveStreamer) PaneCommand(tileID string) (string, error) { return l.ctrl.PaneCommand(tileID) }

// Manager owns the single live PTY per tile and the takeover semantics. It is
// the namespace-side half of OpenShell; see Acquire.
type Manager struct {
	streamer Streamer
	mu       sync.Mutex
	active   map[string]*entry
}

type entry struct {
	session Session
	stopOld chan struct{} // closed when a takeover evicts the current holder
}

// NewManager wraps a Streamer. A nil Streamer is not allowed; a caller with no
// shell backend leaves the Manager itself nil.
func NewManager(s Streamer) *Manager {
	return &Manager{streamer: s, active: map[string]*entry{}}
}

// Acquire returns the live session for tileID. An active holder is taken over:
// signalled to exit, its PTY reused, and its screen repainted for the new
// holder. With no holder, a live tmux session is attached and a dead one is
// created if allowCreate, else ErrSessionGone.
//
// allowCreate is the caller's intent, false for a snapshotted tile where
// fabricating state behind the JPEG would be wrong. internal/local's OpenShell
// derives it from the tile's PreviewBlobID. The returned channel closes when a
// later Acquire takes over, so a caller pumps until it, the session's Done, or
// the end of the request.
func (m *Manager) Acquire(tileID string, allowCreate bool, cols, rows uint16) (Session, chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.active[tileID]; ok {
		close(e.stopOld)
		e.stopOld = make(chan struct{})
		// The PTY is reused, so tmux cannot see that the viewer changed and
		// nothing repaints: the new pane's empty terminal would stay blank
		// until something else happened to resize it. Bounce the winsize one
		// row taller and back, because the kernel raises SIGWINCH only on a
		// real change. Height only, so nothing rewraps on the way through.
		_ = e.session.Resize(cols, rows+1)
		_ = e.session.Resize(cols, rows)
		return e.session, e.stopOld, nil
	}

	alive, err := m.streamer.HasSession(tileID)
	if err != nil {
		return nil, nil, fmt.Errorf("shellsvc: probe tile %s: %w", tileID, err)
	}
	mode := tmux.ModeAttach
	if !alive {
		if !allowCreate {
			return nil, nil, ErrSessionGone
		}
		mode = tmux.ModeCreate
	}
	sess, err := m.streamer.OpenSession(tileID, mode, cols, rows)
	if err != nil {
		return nil, nil, err
	}
	m.active[tileID] = &entry{session: sess, stopOld: make(chan struct{})}
	return sess, m.active[tileID].stopOld, nil
}

// Release is the inverse of Acquire. If this holder still owns the entry, so
// no takeover happened mid-flight, it closes the PTY-side session, which kills
// the gridwell-spawned tmux client but leaves the tmux server and the shell
// running for the next refresh, then fires onDetach, the best-effort title
// capture. On takeover it is a no-op: the new holder keeps the session.
func (m *Manager) Release(tileID string, mySession Session, myStopOld chan struct{}, onDetach func()) {
	m.mu.Lock()
	e, ok := m.active[tileID]
	matches := ok && e.session == mySession && e.stopOld == myStopOld
	if matches {
		delete(m.active, tileID)
	}
	m.mu.Unlock()
	if matches {
		log.Printf("[shellsvc] detach tile=%s", tileID)
		_ = mySession.Close()
		if onDetach != nil {
			onDetach()
		}
	}
}

// HasSession reports whether the tile's tmux session exists right now.
func (m *Manager) HasSession(tileID string) (bool, error) { return m.streamer.HasSession(tileID) }

// Kill removes the tile's tmux session. Idempotent.
func (m *Manager) Kill(tileID string) error { return m.streamer.Kill(tileID) }

// PaneCommand returns the foreground command of the tile's session, or "" if
// it is gone. It labels a frozen shell on detach.
func (m *Manager) PaneCommand(tileID string) (string, error) { return m.streamer.PaneCommand(tileID) }

// CleanupOrphans kills tmux sessions whose tile id no longer exists, the
// bounded leak left by a delete that raced a crash. exists is queried per live
// session. It is best-effort: a per-session failure does not abort the pass,
// and the count killed comes back alongside the first error.
func (m *Manager) CleanupOrphans(_ context.Context, exists func(tileID string) (bool, error)) (int, error) {
	live, err := m.streamer.ListLiveTileIDs()
	if err != nil {
		return 0, fmt.Errorf("list live sessions: %w", err)
	}
	killed := 0
	var firstErr error
	for _, id := range live {
		ok, err := exists(id)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("exists tile %s: %w", id, err)
			}
			continue
		}
		if ok {
			continue
		}
		if err := m.streamer.Kill(id); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("kill orphan tile %s: %w", id, err)
			}
			continue
		}
		killed++
	}
	return killed, firstErr
}

// ClampSize clamps a requested cols and rows to the PTY minimums,
// substituting the shell defaults for zero or too-small values.
func ClampSize(cols, rows uint16) (uint16, uint16) {
	if cols == 0 {
		cols = DefaultCols
	} else if cols < MinCols {
		cols = MinCols
	}
	if rows == 0 {
		rows = DefaultRows
	} else if rows < MinRows {
		rows = MinRows
	}
	return cols, rows
}
