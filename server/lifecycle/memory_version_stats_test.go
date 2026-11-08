package lifecycle

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestMemoryByteChanges(t *testing.T) {
	for _, tt := range []struct {
		name, before, after string
		added, removed      uint64
	}{
		{"unchanged", "same\n", "same\n", 0, 0},
		{"empty", "", "", 0, 0},
		{"insert", "keep\n", "keep\n새🙂\n", 8, 0},
		{"delete", "keep\n새🙂\n", "keep\n", 0, 8},
		{"character within paragraph", "unchanged old tail", "unchanged odd tail", 1, 1},
		{"Korean shared UTF8 prefix", "앞 가 뒤", "앞 각 뒤", 3, 3},
		{"emoji shared UTF8 prefix", "before 🙂 after", "before 🙃 after", 4, 4},
		{"separate edits", "old keep 가 tail", "odd keep 각 tail", 4, 4},
		{"blank line", "", "\n", 1, 0},
		{"remove blank", "one\n\n", "one\n", 0, 1},
		{"delete body", "한글\n🙂", "", 0, 11},
		{"EOF newline added", "one", "one\n", 1, 0},
		{"EOF newline removed", "one\n", "one", 0, 1},
		{"CRLF", "one\r\ntwo\r\n", "one\ntwo\n", 0, 2},
		{"literal diff markup", "keep\n", "keep\n+++ literal\n", 12, 0},
		{"large paragraph", strings.Repeat("unchanged ", 10000) + "가" + strings.Repeat(" tail", 10000), strings.Repeat("unchanged ", 10000) + "각" + strings.Repeat(" tail", 10000), 3, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			added, removed := memoryByteChanges(tt.before, tt.after)
			if added != tt.added || removed != tt.removed {
				t.Fatalf("got +%dB -%dB, want +%dB -%dB", added, removed, tt.added, tt.removed)
			}
			if int64(added)-int64(removed) != int64(len(tt.after))-int64(len(tt.before)) {
				t.Fatal("byte changes do not account for body size change")
			}
		})
	}
}

func TestMemoryLineChanges(t *testing.T) {
	for _, tt := range []struct {
		name, before, after string
		added, removed      uint32
	}{
		{"unchanged", "same\n", "same\n", 0, 0},
		{"empty", "", "", 0, 0},
		{"insert", "keep\n", "keep\nnew\nmore\n", 2, 0},
		{"delete", "keep\nold\n", "keep\n", 0, 1},
		{"replace", "keep\nold\ntail\n", "keep\nnew\nextra\ntail\n", 2, 1},
		{"empty to blank", "", "\n", 1, 0},
		{"remove blank", "one\n\n", "one\n", 0, 1},
		{"delete body", "one\ntwo", "", 0, 2},
		{"EOF newline added", "one", "one\n", 1, 1},
		{"EOF newline removed", "one\n", "one", 1, 1},
		{"CRLF", "one\r\ntwo\r\n", "one\ntwo\n", 2, 2},
		{"Unicode", "한글\nold\n✓\n", "한글\n새 내용\n✓\n", 1, 1},
		{"literal diff headers", "keep\n", "keep\n+++ literal\n--- literal\n@@ literal\n", 3, 0},
		{"separate edits", "a\nold\nkeep\nother\nz\n", "a\nnew\nkeep\nchanged\nz\n", 2, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			added, removed := memoryLineChanges(tt.before, tt.after)
			if added != tt.added || removed != tt.removed {
				t.Fatalf("got +%d -%d, want +%d -%d", added, removed, tt.added, tt.removed)
			}
		})
	}
}

func TestMemoryVersionStatsPersistAcrossMetadataAndPagination(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := t.Context()
	s := stack.Memory()
	m, err := s.Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), Name: "stats.md", Content: "keep\nold\n"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	update := func(content string) {
		t.Helper()
		m, err = s.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Content: ptr(content), BaseRevision: ptr(m.GetRevision()), DateUpdated: m.GetDateUpdated()}.Build())
		if err != nil {
			t.Fatal(err)
		}
	}
	update("keep\nnew\nadded\n")
	m, err = s.Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Name: ptr("renamed.md"), Labels: map[string]string{"topic": "test"}, DateUpdated: m.GetDateUpdated()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	update(m.GetContent()) // A body no-op must keep revision 2 and its original stats.
	if m.GetRevision() != 2 {
		t.Fatal(m)
	}
	if _, err = s.Erase(ctx, m.Ref()); err != nil {
		t.Fatal(err)
	}
	trash, err := s.Get(ctx, resource.MemoryGetRequest_builder{Ref: m.Ref(), Select: memoryAll()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	m, err = s.Restore(ctx, resource.MemoryRestoreRequest_builder{Ref: m.Ref(), DateUpdated: trash.GetDateUpdated()}.Build())
	if err != nil {
		t.Fatal(err)
	}
	update("")
	update("keep\nold\n")
	page, err := s.Versions(ctx, resource.MemoryVersionsRequest_builder{Ref: m.Ref(), Size: 2}.Build())
	if err != nil || len(page.GetVersions()) != 2 || page.GetNextRevision() != 3 {
		t.Fatal(page, err)
	}
	check := func(v *resource.MemoryVersion, revision uint64, added, removed uint32, bytesAdded, bytesRemoved uint64) {
		t.Helper()
		if v.GetRevision() != revision || !v.HasLinesAdded() || !v.HasLinesRemoved() || v.GetLinesAdded() != added || v.GetLinesRemoved() != removed {
			t.Fatal(v)
		}
		if !v.HasBytesAdded() || !v.HasBytesRemoved() || v.GetBytesAdded() != bytesAdded || v.GetBytesRemoved() != bytesRemoved {
			t.Fatal(v)
		}
		data, e := proto.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		var decoded resource.MemoryVersion
		if e = proto.Unmarshal(data, &decoded); e != nil || !proto.Equal(v, &decoded) {
			t.Fatal(&decoded, e)
		}
	}
	check(page.GetVersions()[0], 4, 2, 0, 9, 0)
	check(page.GetVersions()[1], 3, 0, 3, 0, 15)
	encoded, err := protojson.Marshal(page.GetVersions()[1])
	if err != nil || !strings.Contains(string(encoded), `"linesAdded":0`) || !strings.Contains(string(encoded), `"bytesAdded":"0"`) {
		t.Fatal(string(encoded), err)
	}
	page, err = s.Versions(ctx, resource.MemoryVersionsRequest_builder{Ref: m.Ref(), Size: 2, BeforeRevision: page.GetNextRevision()}.Build())
	if err != nil || len(page.GetVersions()) != 2 || page.GetNextRevision() != 0 {
		t.Fatal(page, err)
	}
	check(page.GetVersions()[0], 2, 2, 1, 8, 2)
	first := page.GetVersions()[1]
	if first.GetRevision() != 1 || first.HasLinesAdded() || first.HasLinesRemoved() || first.HasBytesAdded() || first.HasBytesRemoved() {
		t.Fatal(first)
	}
	encoded, err = protojson.Marshal(first)
	if err != nil || strings.Contains(string(encoded), "linesAdded") || strings.Contains(string(encoded), "linesRemoved") || strings.Contains(string(encoded), "bytesAdded") || strings.Contains(string(encoded), "bytesRemoved") {
		t.Fatal(string(encoded), err)
	}
	var hash string
	var added, removed sql.NullInt64
	if err = db.QueryRowContext(ctx, `SELECT content_hash,lines_added,lines_removed FROM memory_version WHERE memory_id=? AND revision=2`, projectid.String(m.GetId())).Scan(&hash, &added, &removed); err != nil || !added.Valid || added.Int64 != 2 || !removed.Valid || removed.Int64 != 1 {
		t.Fatal(hash, added, removed, err)
	}
	if hash != fmt.Sprintf("%x", sha256.Sum256([]byte("keep\nnew\nadded\n"))) {
		t.Fatal(hash)
	}
}

func TestMemoryByteStatsFailureRollsBackBodyAuditAndChanges(t *testing.T) {
	f, stack, db := taskStack(t)
	ctx := t.Context()
	m, err := stack.Memory().Add(ctx, resource.MemoryAddRequest_builder{Project: projectRef(f.p.Id), Name: "rollback.md", Content: "old\n"}.Build())
	if err != nil {
		t.Fatal(err)
	}
	var audits, changes int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_change`).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `CREATE TRIGGER reject_memory_stats BEFORE INSERT ON memory_version WHEN NEW.revision=2 AND NEW.bytes_added>0 BEGIN SELECT RAISE(ABORT,'stats rejected'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = stack.Memory().Patch(ctx, resource.MemoryPatchRequest_builder{Ref: m.Ref(), Content: ptr("new\n"), BaseRevision: ptr(uint64(1)), DateUpdated: m.GetDateUpdated()}.Build())
	if err == nil {
		t.Fatal("version stats failure committed")
	}
	var revision int
	var content string
	if err = db.QueryRowContext(ctx, `SELECT revision,content FROM memory WHERE id=?`, projectid.String(m.GetId())).Scan(&revision, &content); err != nil || revision != 1 || content != "old\n" {
		t.Fatal(revision, content, err)
	}
	for query, want := range map[string]int{`SELECT COUNT(*) FROM memory_version`: 1, `SELECT COUNT(*) FROM memory_change`: changes, `SELECT COUNT(*) FROM audit`: audits} {
		var got int
		if err = db.QueryRowContext(ctx, query).Scan(&got); err != nil || got != want {
			t.Fatal(query, got, want, err)
		}
	}
}

func memoryStatsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "stats.db"), MaxOpenConns: 1}).Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`CREATE TABLE memory_version(memory_id TEXT NOT NULL,revision INTEGER NOT NULL,content TEXT NOT NULL,content_hash TEXT NOT NULL,date_created DATETIME NOT NULL,PRIMARY KEY(memory_id,revision))`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMemoryLineStatsMigrationBackfillsAndPreservesVersions(t *testing.T) {
	db := memoryStatsDB(t)
	ctx := t.Context()
	created := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	insert := func(id string, revision int, content string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `INSERT INTO memory_version(memory_id,revision,content,content_hash,date_created) VALUES(?,?,?,?,?)`, id, revision, content, fmt.Sprintf("hash-%d", revision), created); err != nil {
			t.Fatal(err)
		}
	}
	for revision := 1; revision <= 40; revision++ {
		insert("one", revision, fmt.Sprintf("keep\n%d\n", revision))
	}
	insert("empty", 1, "")
	insert("empty", 2, "\n")
	insert("gap", 3, "not revision 2\n")
	for run := 0; run < 3; run++ {
		if run == 1 {
			insert("one", 41, "keep\n41\n")
		} // Older binaries leave newly added columns NULL.
		if err := migrateMemoryVersionStats(ctx, db); err != nil {
			t.Fatal(err)
		}
		rows, err := db.QueryContext(ctx, `SELECT memory_id,revision,content,content_hash,date_created,lines_added,lines_removed,bytes_added,bytes_removed FROM memory_version ORDER BY memory_id,revision`)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for rows.Next() {
			var id, content, hash string
			var revision int
			var date time.Time
			var added, removed, bytesAdded, bytesRemoved sql.NullInt64
			if err = rows.Scan(&id, &revision, &content, &hash, &date, &added, &removed, &bytesAdded, &bytesRemoved); err != nil {
				t.Fatal(err)
			}
			count++
			if hash != fmt.Sprintf("hash-%d", revision) || !date.Equal(created) {
				t.Fatal(id, revision, hash, date)
			}
			if revision == 1 || id == "gap" {
				if added.Valid || removed.Valid || bytesAdded.Valid || bytesRemoved.Valid {
					t.Fatal(id, revision, added, removed)
				}
				continue
			}
			wantAdded, wantRemoved := int64(1), int64(1)
			if id == "empty" {
				wantRemoved = 0
				if content != "\n" {
					t.Fatal(content)
				}
			} else if content != fmt.Sprintf("keep\n%d\n", revision) {
				t.Fatal(content)
			}
			if !added.Valid || !removed.Valid || added.Int64 != wantAdded || removed.Int64 != wantRemoved {
				t.Fatal(id, revision, added, removed)
			}
			wantBytesAdded, wantBytesRemoved := uint64(1), uint64(0)
			if id != "empty" {
				wantBytesAdded, wantBytesRemoved = memoryByteChanges(fmt.Sprintf("keep\n%d\n", revision-1), content)
			}
			if !bytesAdded.Valid || !bytesRemoved.Valid || uint64(bytesAdded.Int64) != wantBytesAdded || uint64(bytesRemoved.Int64) != wantBytesRemoved {
				t.Fatal(id, revision, bytesAdded, bytesRemoved)
			}
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		want := 43
		if run > 0 {
			want = 44
		}
		if count != want {
			t.Fatal(count, want)
		}
	}
}

func TestMemoryLineStatsMigrationRepairsPartialSchemaAndRollsBackOnFailure(t *testing.T) {
	db := memoryStatsDB(t)
	ctx := t.Context()
	for _, q := range []string{
		`ALTER TABLE memory_version ADD COLUMN lines_added INTEGER`,
		`INSERT INTO memory_version(memory_id,revision,content,content_hash,date_created,lines_added) VALUES('one',1,'a','a',CURRENT_TIMESTAMP,NULL),('one',2,'b','b',CURRENT_TIMESTAMP,99)`,
		`CREATE TRIGGER reject_stats_backfill BEFORE UPDATE ON memory_version BEGIN SELECT RAISE(ABORT,'backfill rejected'); END`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateMemoryVersionStats(ctx, db); err == nil {
		t.Fatal("migration failure was ignored")
	}
	// Adding the second column and any partial backfill must roll back together.
	for _, column := range []string{"lines_removed", "bytes_added", "bytes_removed"} {
		if _, err := db.ExecContext(ctx, `SELECT `+column+` FROM memory_version`); err == nil {
			t.Fatalf("partial schema migration committed: %s", column)
		}
	}
	var added int
	if err := db.QueryRowContext(ctx, `SELECT lines_added FROM memory_version WHERE revision=2`).Scan(&added); err != nil || added != 99 {
		t.Fatal(added, err)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER reject_stats_backfill`); err != nil {
		t.Fatal(err)
	}
	if err := migrateMemoryVersionStats(ctx, db); err != nil {
		t.Fatal(err)
	}
	var removed int
	if err := db.QueryRowContext(ctx, `SELECT lines_added,lines_removed FROM memory_version WHERE revision=2`).Scan(&added, &removed); err != nil || added != 1 || removed != 1 {
		t.Fatal(added, removed, err)
	}
}

func TestMemoryByteStatsMigrationPreservesExistingLineStatistics(t *testing.T) {
	db := memoryStatsDB(t)
	ctx := t.Context()
	for _, q := range []string{
		`ALTER TABLE memory_version ADD COLUMN lines_added INTEGER`,
		`ALTER TABLE memory_version ADD COLUMN lines_removed INTEGER`,
		`INSERT INTO memory_version(memory_id,revision,content,content_hash,date_created,lines_added,lines_removed) VALUES('one',1,'앞 가 뒤','first',CURRENT_TIMESTAMP,NULL,NULL),('one',2,'앞 각 뒤','second',CURRENT_TIMESTAMP,7,8)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateMemoryVersionStats(ctx, db); err != nil {
		t.Fatal(err)
	}
	var la, lr, ba, br int
	if err := db.QueryRowContext(ctx, `SELECT lines_added,lines_removed,bytes_added,bytes_removed FROM memory_version WHERE revision=2`).Scan(&la, &lr, &ba, &br); err != nil || la != 7 || lr != 8 || ba != 3 || br != 3 {
		t.Fatal(la, lr, ba, br, err)
	}
}
