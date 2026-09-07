// Package shelldriver spawns a process attached to a PTY and bridges its
// stdin and stdout to a caller-supplied I/O surface. The shell door wraps a
// Session in a duplex transport; tests substitute in-memory transports to
// exercise the driver without a real WebSocket.
//
// One Session is one PTY is one spawned process. In practice the spawned
// process is `tmux new-session` or `tmux attach-session`, but the driver
// does not know that: it execs the configured binary with the configured
// args, and the mapping lives above it.
//
// A PTY is a unix facility. shelldriver_unix.go is the real driver;
// shelldriver_nopty.go is what every other platform gets — a Start that
// refuses with ErrShellsUnavailable, so a Windows build carries no
// creack/pty dependency and no half-working PTY. This file holds what both
// halves share.
package shelldriver

import "errors"

// ErrShellsUnavailable is what Start returns on a platform with no PTY. It
// is a STATE, not a fault: shells-off is already a coherent shape for a node
// (server.yaml disable_shells), and a platform that cannot host a PTY
// presents the same way — the attach is refused with a reason, which the
// shell door turns into the client's exit message exactly as it does a dead
// tmux session or a failed exec. Nothing here logs and returns.
var ErrShellsUnavailable = errors.New("shell tiles are unavailable on this node: no PTY on this platform")

// Config describes how a Session should be started.
type Config struct {
	// Cwd is the directory bash should start in. If empty, the driver
	// falls back to $HOME, then to the parent process's working dir.
	Cwd string
	// Cols / Rows is the initial PTY window size in character cells.
	// Both must be > 0; the caller is expected to pass real terminal
	// dimensions (the server reads them off the client's pane size).
	Cols, Rows uint16
	// BashPath is the bash binary to exec. Empty means "bash" looked up
	// on $PATH. Tests substitute a fake shell.
	BashPath string
	// Args overrides the bash command-line arguments. Empty defaults to
	// {"-i"} (interactive shell that sources the user's rc files). Tests
	// pass {"--norc", "--noprofile", "-i"} so the prompt and environment
	// are deterministic.
	Args []string
	// Env, if non-nil, overrides the spawned process's environment.
	// Empty leaves it as os.Environ().
	Env []string
}
