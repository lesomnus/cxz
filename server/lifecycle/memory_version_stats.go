package lifecycle

import (
	"context"
	"database/sql"

	"github.com/aymanbagabas/go-udiff"
	"github.com/pmezard/go-difflib/difflib"
)

// Diff at rune boundaries so a changed Korean character or emoji contributes
// its full UTF-8 length, without counting the rest of its unchanged paragraph.
func memoryByteChanges(before, after string) (added, removed uint64) {
	for _, edit := range udiff.Strings(before, after) {
		added += uint64(len(edit.New))
		removed += uint64(edit.End - edit.Start)
	}
	return
}

// Use the same line matching as Read's unified diff, without counting headers,
// context or the EOF marker as content lines. A newline-only change replaces
// its last line, just as it does in the displayed diff.
func memoryLineChanges(before, after string) (added, removed uint32) {
	for _, op := range difflib.NewMatcher(memoryDiffLines(before), memoryDiffLines(after)).GetOpCodes() {
		switch op.Tag {
		case 'r':
			added += uint32(op.J2 - op.J1)
			removed += uint32(op.I2 - op.I1)
		case 'i':
			added += uint32(op.J2 - op.J1)
		case 'd':
			removed += uint32(op.I2 - op.I1)
		}
	}
	return
}

// Old versions retain their full bodies, so this migration can derive the same
// statistics as a new write. NULL means no comparison is available, not zero.
// The partial index also catches rows written by an older binary after an
// upgrade, without rescanning all retained bodies on every subsequent startup.
func migrateMemoryVersionStats(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(memory_version)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primary int
		var name, kind string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primary); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, column := range []string{"lines_added", "lines_removed", "bytes_added", "bytes_removed"} {
		if !columns[column] {
			if _, err = tx.ExecContext(ctx, `ALTER TABLE memory_version ADD COLUMN `+column+` INTEGER CHECK (`+column+` >= 0)`); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS memory_version_missing_stats ON memory_version(memory_id,revision) WHERE revision>1 AND (lines_added IS NULL OR lines_removed IS NULL OR bytes_added IS NULL OR bytes_removed IS NULL)`); err != nil {
		return err
	}
	for {
		rows, err = tx.QueryContext(ctx, `SELECT v.memory_id,v.revision,b.content,v.content,v.lines_added,v.lines_removed,v.bytes_added,v.bytes_removed FROM memory_version v JOIN memory_version b ON b.memory_id=v.memory_id AND b.revision=v.revision-1 WHERE v.revision>1 AND (v.lines_added IS NULL OR v.lines_removed IS NULL OR v.bytes_added IS NULL OR v.bytes_removed IS NULL) ORDER BY v.memory_id,v.revision LIMIT 32`)
		if err != nil {
			return err
		}
		type pendingVersion struct {
			id                                                 string
			revision                                           uint64
			before, after                                      string
			linesAdded, linesRemoved, bytesAdded, bytesRemoved sql.NullInt64
		}
		var pending []pendingVersion
		for rows.Next() {
			var v pendingVersion
			if err = rows.Scan(&v.id, &v.revision, &v.before, &v.after, &v.linesAdded, &v.linesRemoved, &v.bytesAdded, &v.bytesRemoved); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			break
		}
		for _, v := range pending {
			if !v.linesAdded.Valid || !v.linesRemoved.Valid {
				added, removed := memoryLineChanges(v.before, v.after)
				v.linesAdded = sql.NullInt64{Int64: int64(added), Valid: true}
				v.linesRemoved = sql.NullInt64{Int64: int64(removed), Valid: true}
			}
			if !v.bytesAdded.Valid || !v.bytesRemoved.Valid {
				added, removed := memoryByteChanges(v.before, v.after)
				v.bytesAdded = sql.NullInt64{Int64: int64(added), Valid: true}
				v.bytesRemoved = sql.NullInt64{Int64: int64(removed), Valid: true}
			}
			if _, err = tx.ExecContext(ctx, `UPDATE memory_version SET lines_added=?,lines_removed=?,bytes_added=?,bytes_removed=? WHERE memory_id=? AND revision=?`, v.linesAdded, v.linesRemoved, v.bytesAdded, v.bytesRemoved, v.id, v.revision); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
