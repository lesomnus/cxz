package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"database/sql"
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
		if err := driver.Exec(ctx, `INSERT INTO memory_version(memory_id,revision,content,content_hash,date_created) VALUES(?,?,?,?,?) ON CONFLICT(memory_id,revision) DO NOTHING`, []any{id, m.GetRevision(), m.GetContent(), hex.EncodeToString(hash[:]), time.Now().UTC()}, nil); err != nil {
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
		`CREATE TABLE IF NOT EXISTS memory_version(memory_id TEXT NOT NULL,revision INTEGER NOT NULL,content TEXT NOT NULL,content_hash TEXT NOT NULL,date_created DATETIME NOT NULL,PRIMARY KEY(memory_id,revision))`,
		`CREATE TABLE IF NOT EXISTS memory_change(sequence INTEGER PRIMARY KEY AUTOINCREMENT,memory_id TEXT NOT NULL,project_id TEXT NOT NULL,revision INTEGER NOT NULL,kind TEXT NOT NULL,date_created DATETIME NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS memory_change_project ON memory_change(project_id,sequence)`,
		`CREATE TABLE IF NOT EXISTS memory_migration(source TEXT PRIMARY KEY,memory_id TEXT NOT NULL,content_hash TEXT NOT NULL,date_created DATETIME NOT NULL)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

var _ bare.SnapshotRecorder = memoryVersionRecorder{}
