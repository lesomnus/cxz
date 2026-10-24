package lifecycle

import (
	"context"
	"database/sql"
)

// The old NOT NULL column must be removed before new rows can be inserted.
// IDs and foreign keys already contain the resource UUID and stay untouched.
func migrateProjectSchema(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(project)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, required, key int
		var name, typ string
		var value any
		if err = rows.Scan(&cid, &name, &typ, &required, &value, &key); err != nil {
			rows.Close()
			return err
		}
		found = found || name == "runtime_id"
	}
	err = rows.Err()
	rows.Close()
	if err != nil || !found {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DROP INDEX IF EXISTS project_runtime_id"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "ALTER TABLE project DROP COLUMN runtime_id"); err != nil {
		return err
	}
	return tx.Commit()
}
