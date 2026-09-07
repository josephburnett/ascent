// Package pluginmeta records a Gridwell database's durable identity inside the
// database itself. An id is assigned once and stored permanently, in the DB and
// in server.yaml. The node's gridwell.db is the caller, through
// local.OpenVerified, so the server checks on each start that the DB it opens is
// the one server.yaml names, and a lost or rewritten server.yaml can be
// reconciled against the DB.
//
// Three keys live in the storage-only _gridwell_meta table, which the
// descriptor-to-proto drift test does not see, since that covers only grids and
// tiles:
//   - gridwell : a marker identifying the file as a Gridwell DB
//   - id       : the durable routing id, the same as the config id
//   - kind     : which schema the DB holds, the same as the config kind
//
// id and kind are immutable. Once written they are strictly verified, in both
// directions, on every subsequent open. The display name is not stored here. It
// is config-only and freely editable.
package pluginmeta

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	// This package calls sql.Open("sqlite", ...), so it owns the driver
	// registration. Relying on another linked package to import the driver
	// would fail at run time with "unknown driver" in a binary that links
	// pluginmeta without the store.
	_ "modernc.org/sqlite"
)

// Sentinel errors. Callers should use errors.Is to test for them.
var (
	// ErrIDMismatch is returned when a DB already carries a different id than
	// the one configured. The durable identity must never silently change.
	ErrIDMismatch = errors.New("pluginmeta: configured id does not match the one stored in the plugin DB")
	// ErrKindMismatch is returned when a DB already carries a different kind.
	// The kind selects the schema, so opening a DB as the wrong kind is
	// refused.
	ErrKindMismatch = errors.New("pluginmeta: configured kind does not match the one stored in the plugin DB")
	// ErrNotInitialized is returned by Verify when the DB is missing or
	// carries no stored identity. Only Create makes an identity, so a DB that
	// was never created fails loudly instead of silently materializing a
	// fresh, empty store.
	ErrNotInitialized = errors.New("pluginmeta: plugin DB is missing or uninitialized")
)

const (
	keyMarker = "gridwell"
	keyID     = "id"
	keyKind   = "kind"
	// keyLegacyUUID is the pre-id-and-kind key. A DB that stored its id under
	// "uuid" is read through this fallback, so its identity is preserved and
	// upgraded to "id" rather than re-minted.
	keyLegacyUUID = "uuid"
	// markerValue is the marker payload. Its value is unimportant; presence is
	// what identifies the file as a Gridwell DB.
	markerValue = "1"
)

// Meta is the durable identity recorded in a DB.
type Meta struct {
	ID   string
	Kind string
}

// Create records (id, kind) as the DB's permanent identity, creating the DB
// file and the marker on first run. It is the only place a DB and its identity
// are born. It is idempotent for the same id and refuses to overwrite a
// different stored id.
func Create(dbPath, id, kind string) error {
	db, err := open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	stored, err := read(db)
	if err != nil {
		return err
	}
	if stored.ID != "" && stored.ID != id {
		return fmt.Errorf("%w (stored %q, configured %q)", ErrIDMismatch, stored.ID, id)
	}
	return writeIdentity(db, id, kind)
}

// Verify strictly checks an existing DB's identity against (id, kind). It never
// creates a DB or a first-run identity: a missing file, or a DB with no stored
// id, is ErrNotInitialized. That is what makes a changed config id fail loudly
// instead of silently opening a fresh store.
//
//   - id == "" and kind == "" → a read-only probe: return whatever is stored
//   - file missing, or no id  → ErrNotInitialized
//   - stored id differs       → ErrIDMismatch
//   - stored kind differs     → ErrKindMismatch
//   - a match, with the older keys upgraded in place → ok
func Verify(dbPath, id, kind string) (Meta, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return Meta{}, fmt.Errorf("%w (%s)", ErrNotInitialized, dbPath)
	}
	db, err := open(dbPath)
	if err != nil {
		return Meta{}, err
	}
	defer db.Close()
	stored, err := read(db)
	if err != nil {
		return Meta{}, err
	}

	// Read-only probe: report what is stored without asserting anything.
	if id == "" && kind == "" {
		return stored, nil
	}
	if stored.ID == "" {
		return Meta{}, fmt.Errorf("%w (%s)", ErrNotInitialized, dbPath)
	}

	// An older DB may carry an id under the uuid key and no kind at all. An
	// empty stored kind was never recorded, so it is adopted rather than
	// refused.
	if stored.ID != id {
		return Meta{}, fmt.Errorf("%w (stored %q, configured %q)", ErrIDMismatch, stored.ID, id)
	}
	if stored.Kind != "" && stored.Kind != kind {
		return Meta{}, fmt.Errorf("%w (stored %q, configured %q)", ErrKindMismatch, stored.Kind, kind)
	}
	// Upgrade an older DB in place. The id may have been readable only through
	// the uuid key, and the kind may not have been recorded at all.
	if err := writeIdentity(db, id, kind); err != nil {
		return Meta{}, err
	}
	return Meta{ID: id, Kind: kind}, nil
}

// open opens the DB and ensures the storage-only identity table exists.
func open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("pluginmeta open %s: %w", dbPath, err)
	}
	// One connection is the single-writer discipline every SQLite handle in
	// this repo pins, so a pooled second connection cannot race the file lock
	// into an instant SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS _gridwell_meta (k TEXT PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("pluginmeta schema: %w", err)
	}
	return db, nil
}

// writeIdentity upserts the marker + id + kind.
func writeIdentity(db *sql.DB, id, kind string) error {
	for _, kv := range []struct{ k, v string }{
		{keyMarker, markerValue},
		{keyID, id},
		{keyKind, kind},
	} {
		if _, err := db.Exec(
			`INSERT INTO _gridwell_meta (k, v) VALUES (?, ?)
			 ON CONFLICT(k) DO UPDATE SET v = excluded.v`,
			kv.k, kv.v); err != nil {
			return fmt.Errorf("pluginmeta write %s: %w", kv.k, err)
		}
	}
	return nil
}

// read returns the stored identity, falling back to the uuid key when the
// canonical id key is absent.
func read(db *sql.DB) (Meta, error) {
	id, err := readKey(db, keyID)
	if err != nil {
		return Meta{}, err
	}
	if id == "" {
		if id, err = readKey(db, keyLegacyUUID); err != nil {
			return Meta{}, err
		}
	}
	kind, err := readKey(db, keyKind)
	if err != nil {
		return Meta{}, err
	}
	return Meta{ID: id, Kind: kind}, nil
}

func readKey(db *sql.DB, k string) (string, error) {
	var v string
	switch err := db.QueryRow(`SELECT v FROM _gridwell_meta WHERE k = ?`, k).Scan(&v); {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("pluginmeta read %s: %w", k, err)
	}
	return v, nil
}
