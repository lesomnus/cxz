package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"database/sql"
	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/cxz/server/bare"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
)

// Bodies and discovery records commit in the generated write's transaction.
// No second connection or process-local mutex can provide this guarantee.
type memoryVersionRecorder struct{}

func (memoryVersionRecorder) Snapshots(method string) bare.SnapshotOptions {
	if !strings.HasPrefix(method, "/cxz.MemoryService/") {
		return bare.SnapshotOptions{}
	}
	return bare.SnapshotOptions{Previous: true, Value: true}
}
func (memoryVersionRecorder) Record(ctx context.Context, s bare.Server, c bare.Change) error {
	if !strings.HasPrefix(c.By, "/cxz.MemoryService/") {
		return nil
	}
	m, _ := c.Value.(*resource.Memory)
	kind := "updated"
	if m == nil {
		m, _ = c.Previous.(*resource.Memory)
		kind = "deleted"
	}
	if m == nil {
		return fmt.Errorf("memory write did not capture its value")
	}
	id := projectid.String(m.GetId())
	driver := s.Db.Driver()
	if kind != "deleted" {
		hash := sha256.Sum256([]byte(m.GetContent()))
		var added, removed any
		if before, ok := c.Previous.(*resource.Memory); ok && m.GetRevision() > 1 && before.GetRevision() == m.GetRevision()-1 {
			added, removed = memoryLineChanges(before.GetContent(), m.GetContent())
		}
		if err := driver.Exec(ctx, `INSERT INTO memory_version(memory_id,revision,content,content_hash,date_created,lines_added,lines_removed) VALUES(?,?,?,?,?,?,?) ON CONFLICT(memory_id,revision) DO NOTHING`, []any{id, m.GetRevision(), m.GetContent(), hex.EncodeToString(hash[:]), time.Now().UTC(), added, removed}, nil); err != nil {
			return err
		}
		rows := &entsql.Rows{}
		if err := driver.Query(ctx, `SELECT content_hash FROM memory_version WHERE memory_id=? AND revision=?`, []any{id, m.GetRevision()}, rows); err != nil {
			return err
		}
		var stored string
		if !rows.Next() {
			rows.Close()
			return fmt.Errorf("missing memory version")
		}
		err := rows.Scan(&stored)
		rows.Close()
		if err != nil {
			return err
		}
		if stored != hex.EncodeToString(hash[:]) {
			return fmt.Errorf("memory revision cannot be overwritten")
		}
		if c.Previous == nil {
			kind = "created"
		}
		if m.HasDateDeleted() {
			kind = "deleted"
		}
	} else {
		if err := driver.Exec(ctx, `DELETE FROM memory_version WHERE memory_id=?`, []any{id}, nil); err != nil {
			return err
		}
	}
	return driver.Exec(ctx, `INSERT INTO memory_change(memory_id,project_id,revision,kind,date_created) VALUES(?,?,?,?,?)`, []any{id, projectid.String(m.GetProject().GetId()), m.GetRevision(), kind, time.Now().UTC()}, nil)
}

func memoryVersionSchema(ctx context.Context, db *sql.DB) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS memory_meta(key TEXT PRIMARY KEY,value TEXT NOT NULL)`,
		`INSERT OR IGNORE INTO memory_meta VALUES('changes_floor','0')`,
		`CREATE TABLE IF NOT EXISTS memory_version(memory_id TEXT NOT NULL,revision INTEGER NOT NULL,content TEXT NOT NULL,content_hash TEXT NOT NULL,date_created DATETIME NOT NULL,lines_added INTEGER CHECK(lines_added>=0),lines_removed INTEGER CHECK(lines_removed>=0),PRIMARY KEY(memory_id,revision))`,
		`CREATE TABLE IF NOT EXISTS memory_change(sequence INTEGER PRIMARY KEY AUTOINCREMENT,memory_id TEXT NOT NULL,project_id TEXT NOT NULL,revision INTEGER NOT NULL,kind TEXT NOT NULL,date_created DATETIME NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS memory_change_project ON memory_change(project_id,sequence)`,
		`CREATE TABLE IF NOT EXISTS memory_migration(source TEXT PRIMARY KEY,memory_id TEXT NOT NULL,content_hash TEXT NOT NULL,date_created DATETIME NOT NULL)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO memory_meta VALUES('epoch',?)`, uuid.NewString()); err != nil {
		return err
	}
	return migrateMemoryVersionStats(ctx, db)
}

var _ bare.SnapshotRecorder = memoryVersionRecorder{}

// RetainMemories performs physical removal only after the logical deadline.
func (s Layer) RetainMemories(ctx context.Context, now time.Time) (int, error) {
	rows, err := s.shared.db.QueryContext(ctx, `SELECT id FROM memory WHERE date_deleted IS NOT NULL AND date_deleted<=? ORDER BY date_deleted,id LIMIT 128`, now)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, id := range ids {
		uid, err := uuid.Parse(id)
		if err != nil {
			return removed, err
		}
		m, err := s.Next().Memory().Get(ctx, resource.MemoryGetRequest_builder{Ref: resource.MemoryById(uid[:]), Select: memoryAll()}.Build())
		if err != nil {
			return removed, err
		}
		if !m.HasDateDeleted() || m.GetDateDeleted().AsTime().After(now) {
			continue
		}
		if _, err = (MemoryServer{Layer: s}).purgeVersioned(ctx, m); err != nil {
			return removed, err
		}
		removed++
	}
	tx, err := s.shared.db.BeginTx(ctx, nil)
	if err != nil {
		return removed, err
	}
	defer tx.Rollback()
	var cut uint64
	err = tx.QueryRowContext(ctx, `SELECT sequence FROM memory_change ORDER BY sequence DESC LIMIT 1 OFFSET 9999`).Scan(&cut)
	if err == sql.ErrNoRows {
		return removed, nil
	}
	if err != nil {
		return removed, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM memory_change WHERE sequence<?`, cut); err != nil {
		return removed, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE memory_meta SET value=? WHERE key='changes_floor'`, fmt.Sprint(cut-1)); err != nil {
		return removed, err
	}
	return removed, tx.Commit()
}
