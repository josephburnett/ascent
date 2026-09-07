package cli

// The per-home serve lock: one `gridwell serve` per Gridwell home. Two
// servers over the same database would each cache and write independently,
// and SQLite's WAL locking would not stop them. An exclusive flock on
// <home>/serve.lock dies with its holder, so there is no stale-pidfile
// protocol. The file holds the holder's banner, which a conflicting serve
// re-emits as "already serving" so the desktop app connects to the running
// server instead of starting a second one.

// errServeLockHeld reports the conflict with the holder's banner, empty when
// the holder has not written it yet. Both platform halves name it.
type errServeLockHeld struct {
	banner string
}

func (e *errServeLockHeld) Error() string {
	if e.banner == "" {
		return "another gridwell serve is starting up for this home"
	}
	return "another gridwell serve is already running for this home: " + e.banner
}
