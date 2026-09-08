package connection

// The transport's store: what the node remembers about its connections beyond
// server.yaml, which is the learned landing, so a dark remote still has a room
// to show. Everything else is config, read fresh every boot. The rows' shape
// belongs to internal/local/store; this file holds the queries only.
//
// The `deleted` flag is the mirror of the config's retired_names, the one owner
// of retirement, reconciled onto these rows at boot (see New) so route and
// Probe can read "retired" off the row they hold. A retired name never returns
// and its namespace stays reserved, so stored references through it stay
// dangling. A name the config merely stopped declaring is not retired.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// ErrNotFound is the missing-row verdict.
var ErrNotFound = errors.New("connection: not found")

// DB is the connection store.
type DB struct {
	db    *sql.DB
	owned bool // Close closes the handle only when this DB opened it
}

// NewDB binds the connection store to the node's one database handle; a second
// handle on the same SQLite file would meet an instant SQLITE_BUSY. It checks
// that the handle came from an opened store, since one the store never
// migrated would fail at the first Get, long after the wiring mistake.
func NewDB(db *sql.DB) (*DB, error) {
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'connections'`).
		Scan(&n); err != nil {
		return nil, fmt.Errorf("connection: look for the connections table: %w", err)
	}
	if n == 0 {
		return nil, errors.New("connection: no connections table on this handle; " +
			"the node's store owns that DDL, so the handle must come from store.Open")
	}
	return &DB{db: db}, nil
}

// Close closes the handle when this DB opened it; a shared handle is the
// store's.
func (d *DB) Close() error {
	if !d.owned {
		return nil
	}
	return d.db.Close()
}

// Stored is one remembered connection.
type Stored struct {
	Name       string
	RemoteRoot string
	Deleted    bool
}

// Get returns the row for name, or ErrNotFound.
func (d *DB) Get(ctx context.Context, name string) (Stored, error) {
	var r Stored
	var del int
	err := d.db.QueryRowContext(ctx, `SELECT name, remote_root, deleted FROM connections WHERE name = ?`, name).
		Scan(&r.Name, &r.RemoteRoot, &del)
	if errors.Is(err, sql.ErrNoRows) {
		return Stored{}, ErrNotFound
	}
	if err != nil {
		return Stored{}, err
	}
	r.Deleted = del != 0
	return r, nil
}

// List returns every row, tombstones included, by name.
func (d *DB) List(ctx context.Context) ([]Stored, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT name, remote_root, deleted FROM connections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Stored
	for rows.Next() {
		var r Stored
		var del int
		if err := rows.Scan(&r.Name, &r.RemoteRoot, &del); err != nil {
			return nil, err
		}
		r.Deleted = del != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// Ensure creates the row for name if absent (a fresh declaration).
func (d *DB) Ensure(ctx context.Context, name string) error {
	_, err := d.db.ExecContext(ctx, `INSERT OR IGNORE INTO connections (name) VALUES (?)`, name)
	return err
}

// SetRemoteRoot records the learned landing.
func (d *DB) SetRemoteRoot(ctx context.Context, name, root string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE connections SET remote_root = ? WHERE name = ?`, root, name)
	return err
}

// Tombstone retires name forever, creating the row if it never existed: a
// retired_names entry on a fresh store is still a reservation.
func (d *DB) Tombstone(ctx context.Context, name string) error {
	if err := d.Ensure(ctx, name); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, `UPDATE connections SET deleted = 1 WHERE name = ?`, name)
	return err
}

// Revive clears a tombstone the config's retired_names does not hold, bringing
// the mirror back in line with it. Everything else on the row is untouched.
func (d *DB) Revive(ctx context.Context, name string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE connections SET deleted = 0 WHERE name = ?`, name)
	return err
}
