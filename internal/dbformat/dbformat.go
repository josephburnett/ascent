// Package dbformat is the shared on-disk format contract for a Gridwell SQLite
// file, which internal/local/store delegates to. PRAGMA application_id marks
// whose file it is, so a foreign SQLite file is refused instead of misread.
// PRAGMA user_version is the schema generation: a stamp newer than the binary is
// refused, an older one brought forward by the chain. Migrations are additive by
// default, so data written by any released binary stays readable. The contract
// is internal/local/store/CLAUDE.md.
package dbformat

import (
	"context"
	"database/sql"
	"fmt"
)

// Migration is one additive step that brings a DB from version To-1 up to To.
type Migration struct {
	To  int
	Run func(ctx context.Context, tx *sql.Tx) error
}

// EnsureVersion enforces the format contract at Open time, running the pending
// migrations of the ordered chain in one transaction. A foreign application_id
// or a user_version past target is refused. A fresh DB, both pragmas 0, is
// stamped straight at target since the caller's Open already materialized the
// latest shape; that holds only if the caller proves by equivalence test that
// the fresh shape equals the v1 base plus the full chain. An unstamped file
// carrying data is the same case, an unversioned shape being the v1 base.
func EnsureVersion(ctx context.Context, db *sql.DB, appID int64, target int, migs []Migration) error {
	gotApp, err := readPragmaInt(ctx, db, "application_id")
	if err != nil {
		return err
	}
	userVer, err := readPragmaInt(ctx, db, "user_version")
	if err != nil {
		return err
	}

	if gotApp == 0 && userVer == 0 {
		// Both identity stamps go in one transaction; header pragmas are
		// transactional. A crash between them would leave application_id set with
		// user_version 0, and the next Open would run the chain against
		// latest-shape tables, whose ADD COLUMNs then fail forever.
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin: %w", err)
		}
		if err := setPragmaIntTx(ctx, tx, "application_id", appID); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := setPragmaIntTx(ctx, tx, "user_version", int64(target)); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	if gotApp != appID {
		return fmt.Errorf("not a Gridwell database of this kind: application_id %#x, want %#x", gotApp, appID)
	}
	if userVer > int64(target) {
		return fmt.Errorf("stored schema version %d is newer than this binary's %d", userVer, target)
	}
	if userVer == int64(target) {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	for _, m := range migs {
		if m.To <= int(userVer) || m.To > target {
			continue
		}
		if err := m.Run(ctx, tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration to v%d: %w", m.To, err)
		}
	}
	// The stamp rides inside the migration transaction; header pragmas are
	// transactional. Stamping after the commit would persist the DDL without the
	// version recording it, after which every Open re-runs the chain, fails on
	// "duplicate column name" and leaves the file unopenable.
	if err := setPragmaIntTx(ctx, tx, "user_version", int64(target)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// readPragmaInt reads an integer-valued PRAGMA. The name is a trusted literal.
func readPragmaInt(ctx context.Context, db *sql.DB, name string) (int64, error) {
	var v int64
	if err := db.QueryRowContext(ctx, "PRAGMA "+name).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

// setPragmaIntTx writes an integer-valued PRAGMA inside a transaction. Header
// pragmas are transactional in SQLite, which is what makes the stamps atomic.
func setPragmaIntTx(ctx context.Context, tx *sql.Tx, name string, v int64) error {
	_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA %s = %d", name, v))
	return err
}
