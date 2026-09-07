package dbformat

import (
	"context"
	"database/sql"
	"fmt"
)

// setPragmaInt writes an integer-valued PRAGMA outside a transaction. Only
// tests seeding foreign headers need it; production stamps ride setPragmaIntTx.
func setPragmaInt(ctx context.Context, db *sql.DB, name string, v int64) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA %s = %d", name, v))
	return err
}
