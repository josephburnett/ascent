package cli

// The per-home serve lock: one `gridwell serve` per Gridwell home. Two
// servers over the same database would each cache and write independently,
// and SQLite's own locking would not stop them, because WAL allows many
// processes. The mechanism is an exclusive flock on <home>/serve.lock:
// kernel-owned and released the instant the holder dies, so there is no
// stale-pidfile protocol and no cleanup to trust.
//
// The lock file's content is the holder's serve banner, written once the
// listener is up. A conflicting serve re-emits it as "gridwell: already
// serving on …" on stdout before exiting nonzero, so the desktop app, which
// parses banners anyway, connects to the running server instead of starting
// a second one. Lock, discovery, and home resolution have one owner, this
// process; the app never learns what a home is.
//
// flock is a unix facility: servelock_unix.go is the mechanism and
// servelock_windows.go is the platform that has none. This file holds what
// serve.go reads either way.

// errServeLockHeld reports the conflict along with the holder's banner,
// which is empty when the holder has not written it yet or the file is
// unreadable. It belongs to neither half: serve.go classifies with it, so
// a platform without flock still has to name the type it never returns.
type errServeLockHeld struct {
	banner string
}

func (e *errServeLockHeld) Error() string {
	if e.banner == "" {
		return "another gridwell serve is starting up for this home"
	}
	return "another gridwell serve is already running for this home: " + e.banner
}
