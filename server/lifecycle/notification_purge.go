package lifecycle

import (
	"context"
	"github.com/lesomnus/payday/pdid"
)

func (s Layer) purgeSessionAttention(ctx context.Context, id []byte) error {
	key, err := pdid.From(id)
	if err != nil {
		return err
	}
	tx, err := s.shared.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`DELETE FROM notificationreceiptrecord WHERE notification_id IN (SELECT id FROM notificationrecord WHERE source_id=?)`,
		`DELETE FROM notificationrecord WHERE source_id=?`,
		`DELETE FROM sessionread WHERE session_id=?`,
	} {
		if _, err = tx.ExecContext(ctx, query, key.Uuid().String()); err != nil {
			return err
		}
	}
	if s.shared.notifications != nil {
		if _, err = tx.ExecContext(ctx, `DELETE FROM session_notification_cursor WHERE session_id=?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
