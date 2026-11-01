package lifecycle

import (
	"context"
	"database/sql"
)

func migrateSessionSchema(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(session)")
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
		found = found || name == "listed"
	}
	err = rows.Err()
	rows.Close()
	if err != nil || !found {
		return err
	}
	_, err = db.ExecContext(ctx, "ALTER TABLE session DROP COLUMN listed")
	return err
}
