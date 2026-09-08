// Package shellsvc owns the tmux-backed session lifecycle. It belongs to the
// namespace that owns the shell tiles, so live bytes cross the namespace
// interface through OpenShell and a shell in a remote namespace streams over
// the same path. Tile ids here are namespace-local; tmux.SessionName maps one
// to a session name.
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

// Sizing defaults and clamps, exported so the OpenShell binder and the resize
// path agree on one set of bounds.
const (
	MinCols     = 20
	MinRows     = 5
	DefaultCols = 80
	DefaultRows = 24
)

// ErrSessionGone is a snapshotted tile whose tmux session is no longer alive.
// Only the JPEG remains, so the caller hides the refresh button rather than
// fabricate a fresh session behind the snapshot.
var ErrSessionGone = errors.New("shell session no longer alive")

// Session's Output is a channel so a takeover or detach returns at once,
// leaving no goroutine orphaned on a PTY syscall.
type Session interface {
	Output() <-chan []byte
	Write(p []byte) (int, error)
	Resize(cols, rows uint16) error
	Done() <-chan struct{}
	Close() error
}

// Streamer is the tmux and PTY backend, stubbed in tests.
type Streamer interface {
	OpenSession(tileID string, mode tmux.Mode, cols, rows uint16) (Session, error)
	HasSession(tileID string) (bool, error)
	Kill(tileID string) error
	ListLiveTileIDs() ([]string, error)
	PaneCommand(tileID string) (string, error)
}

// NewLive composes the tmux argv through the controller and execs it through
// shelldriver, so a detach kills only the client and leaves the tmux server
// running.
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

// Manager owns the single live PTY per tile and the takeover semantics, the
// namespace-side half of OpenShell.
type Manager struct {
	streamer Streamer
	mu       sync.Mutex
	active   map[string]*entry
}

type entry struct {
	session Session
	stopOld chan struct{} // closed when a takeover evicts the current holder
}

// NewManager requires a non-nil Streamer; a caller with no shell backend
// leaves the Manager itself nil.
func NewManager(s Streamer) *Manager {
	return &Manager{streamer: s, active: map[string]*entry{}}
}

// Acquire takes over an active holder: signalled to exit, its PTY reused, its
// screen repainted. With no holder a live tmux session is attached and a dead
// one created if allowCreate, which is false for a snapshotted tile where
// fabricating state behind the JPEG would be wrong. The returned channel closes
// when a later Acquire takes over.
func (m *Manager) Acquire(tileID string, allowCreate bool, cols, rows uint16) (Session, chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.active[tileID]; ok {
		close(e.stopOld)
		e.stopOld = make(chan struct{})
		// The PTY is reused, so tmux cannot see the viewer changed and the
		// new pane's terminal would stay blank until something resized it.
		// Bounce the winsize one row taller and back, the kernel raising
		// SIGWINCH only on a real change. Height only, so nothing rewraps.
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

// Release closes the PTY-side session if this holder still owns the entry,
// killing the gridwell-spawned tmux client but leaving the tmux server and the
// shell running for the next refresh, then fires onDetach. After a takeover it
// is a no-op: the new holder keeps the session.
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

func (m *Manager) HasSession(tileID string) (bool, error) { return m.streamer.HasSession(tileID) }

// Kill is idempotent.
func (m *Manager) Kill(tileID string) error { return m.streamer.Kill(tileID) }

// PaneCommand labels a frozen shell on detach, "" when the session is gone.
func (m *Manager) PaneCommand(tileID string) (string, error) { return m.streamer.PaneCommand(tileID) }

// CleanupOrphans kills tmux sessions whose tile id no longer exists, the
// bounded leak left by a delete that raced a crash. A per-session failure does
// not abort the pass, and the count killed comes back with the first error.
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

// ClampSize substitutes the shell defaults for zero or too-small values.
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
