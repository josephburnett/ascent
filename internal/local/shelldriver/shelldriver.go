// Package shelldriver spawns a process attached to a PTY. One Session is one
// PTY is one spawned process; that the process is usually `tmux attach-session`
// is known only in internal/local/shellsvc. shelldriver_unix.go is the real
// driver and shelldriver_nopty.go gives every other platform a Start that
// refuses with ErrShellsUnavailable, carrying no creack/pty dependency.
package shelldriver

import "errors"

// ErrShellsUnavailable is a state the client is told about: the shell door
// turns it into an exit message the way it does a dead tmux session, as it
// does for a node that sets disable_shells. Nothing here logs and returns.
var ErrShellsUnavailable = errors.New("shell tiles are unavailable on this node: no PTY on this platform")

type Config struct {
	// Cwd empty, or a path that does not exist, falls back through
	// resolveCwd.
	Cwd string
	// Cols and Rows are the initial PTY window size in cells, both > 0.
	Cols, Rows uint16
	// BashPath is the binary to exec. Empty looks up "bash" on $PATH.
	BashPath string
	// Args empty defaults to {"-i"}, an interactive shell that sources the
	// user's rc files.
	Args []string
	// Env non-nil replaces the environment; nil uses os.Environ() with TERM
	// defaulted.
	Env []string
}
