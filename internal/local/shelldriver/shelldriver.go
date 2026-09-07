// Package shelldriver spawns a process attached to a PTY and bridges its
// stdin and stdout to a caller-supplied I/O surface. One Session is one PTY is
// one spawned process. The driver execs the configured binary with the
// configured args; that the process is usually `tmux new-session` or `tmux
// attach-session` is known only above it, in internal/local/shellsvc.
//
// A PTY is a unix facility. shelldriver_unix.go is the real driver, and
// shelldriver_nopty.go gives every other platform a Start that refuses with
// ErrShellsUnavailable, so a Windows build carries no creack/pty dependency.
// This file holds what both halves share.
package shelldriver

import "errors"

// ErrShellsUnavailable is what Start returns on a platform with no PTY.
// Unavailable shells are a state the client is told about: the attach is
// refused with a reason, which the shell door turns into an exit message the
// same way it does for a dead tmux session. A node that sets disable_shells in
// server.yaml presents the same way. Nothing here logs and returns.
var ErrShellsUnavailable = errors.New("shell tiles are unavailable on this node: no PTY on this platform")

// Config describes how a Session should be started.
type Config struct {
	// Cwd is the directory bash should start in. Empty, or a path that does
	// not exist, falls back through resolveCwd.
	Cwd string
	// Cols and Rows are the initial PTY window size in character cells. Both
	// must be > 0.
	Cols, Rows uint16
	// BashPath is the binary to exec. Empty looks up "bash" on $PATH.
	BashPath string
	// Args overrides the command-line arguments. Empty defaults to {"-i"}, an
	// interactive shell that sources the user's rc files.
	Args []string
	// Env, when non-nil, replaces the spawned process's environment. Nil uses
	// os.Environ() with TERM defaulted.
	Env []string
}
