package historypolicy

import (
	"context"
	"database/sql"
)

// Floors are durable even when out-of-order history requests finish after a trim.
func Floor(ctx context.Context, tx *sql.Tx, id string) (uint64, error) {
	if _, err := tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS history_floors(session_id TEXT PRIMARY KEY, seq INTEGER NOT NULL)"); err != nil {
		return 0, err
	}
	var n uint64
	err := tx.QueryRowContext(ctx, "SELECT seq FROM history_floors WHERE session_id=?", id).Scan(&n)
	if err == sql.ErrNoRows {
		err = nil
	}
	return n, err
}
func AdvanceFloor(ctx context.Context, tx *sql.Tx, id string, n uint64) error {
	old, err := Floor(ctx, tx, id)
	if err != nil || old >= n {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO history_floors VALUES(?,?) ON CONFLICT(session_id) DO UPDATE SET seq=excluded.seq", id, n); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM events WHERE session_id=? AND seq<=?", id, n); err != nil {
		return err
	}
	// Old releases must reject this state rather than resurrect trimmed history.
	_, err = tx.ExecContext(ctx, "PRAGMA user_version=2")
	return err
}
