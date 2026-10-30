package lifecycle

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/internal/ent"
	"github.com/lesomnus/cxz/server/pd"
	"github.com/lesomnus/payday/notify"
	"github.com/lesomnus/payday/pdid"
	"github.com/lesomnus/payday/trail"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
)

// RetainAudit reuses payday's batched, policy-aware deletion. It deliberately
// neither measures audit bytes nor vacuums the shared resource database.
func (s Layer) RetainAudit(ctx context.Context, now time.Time, days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, s.shared.db)))
	p := trail.Policy{Keep: trail.Keep{Retain: time.Duration(days) * 24 * time.Hour, Discard: true}}
	n, _, err := p.Collect(ctx, pd.TrailStore(client), trail.Kinds{}, now.Add(-time.Duration(days)*24*time.Hour))
	return n, err
}

// RetainNotifications keeps the latest unread state of each source/kind and
// recent read history. Cursor/idempotency checkpoints and pending questions
// remain untouched. Private rows are removed together, then watchers invalidate
// their pages through payday's supported notification invalidation path.
func (s Layer) RetainNotifications(ctx context.Context, now time.Time, days int) (int, error) {
	n := s.shared.notifications
	if n == nil || days <= 0 {
		return 0, nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	tx, err := s.shared.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	rows, err := tx.QueryContext(ctx, `SELECT n.id,s.project_id FROM notificationrecord n JOIN session s ON s.id=n.source_id
	 WHERE n.producer='cxz.manager' AND (
	 EXISTS(SELECT 1 FROM notificationreceiptrecord r WHERE r.notification_id=n.id AND r.reader_id=? AND r.data_read IS NOT NULL AND r.data_read<?)
	 OR (NOT EXISTS(SELECT 1 FROM notificationreceiptrecord r WHERE r.notification_id=n.id AND r.reader_id=? AND r.data_read IS NOT NULL)
	 AND EXISTS(SELECT 1 FROM notificationrecord newer WHERE newer.source_id=n.source_id AND newer.kind=n.kind AND newer.topic_key=n.topic_key AND newer.producer=n.producer AND (newer.date_created>n.date_created OR (newer.date_created=n.date_created AND newer.id>n.id))
	 AND NOT EXISTS(SELECT 1 FROM notificationreceiptrecord r WHERE r.notification_id=newer.id AND r.reader_id=? AND r.data_read IS NOT NULL))))
	 ORDER BY n.date_created,n.id LIMIT 128`, n.owner.Uuid(), cutoff, n.owner.Uuid(), n.owner.Uuid())
	if err != nil {
		return 0, err
	}
	var ids []string
	topics := map[string]bool{}
	for rows.Next() {
		var id, topic string
		if err = rows.Scan(&id, &topic); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
		topics[topic] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, "DELETE FROM notificationreceiptrecord WHERE notification_id=?", id); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM notificationrecord WHERE id=?", id); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	if len(ids) > 0 {
		var ts []notify.Topic
		for topic := range topics {
			id, err := pdid.Parse(topic)
			if err != nil {
				return len(ids), err
			}
			ts = append(ts, notify.Topic{Resource: id})
		}
		if err = n.service.Invalidate(n.context(ctx), pdid.Nil, ts); err != nil {
			return len(ids), err
		}
	}
	return len(ids), nil
}
