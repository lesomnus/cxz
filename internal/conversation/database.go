package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

func (s *Store) readDatabase(ctx context.Context, root *os.Root, dir, session string) ([]core.Event, []unreadable, error) {
	// SQLite opens the database and its WAL sidecars by path. Require physical
	// directories and regular files within the same scope as the manifest.
	for _, path := range []string{filepath.Dir(dir), dir} {
		info, err := root.Lstat(path)
		if err != nil {
			return nil, nil, err
		}
		if !info.IsDir() {
			return nil, nil, fmt.Errorf("invalid conversation store directory")
		}
	}
	path := sessionstore.Path(dir)
	for _, name := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		info, err := root.Lstat(name)
		if name != path && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("invalid conversation store file")
		}
	}
	db, err := sessionstore.Open(ctx, filepath.Join(root.Name(), path), session, false)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	// A single SELECT sees one snapshot of the committed records, including
	// those in the WAL. Oversized records can be reported without loading them.
	rows, err := db.DB().QueryContext(ctx, `
		SELECT seq, length(data), CASE WHEN length(data) <= ? THEN data END
		FROM events WHERE session_id = ? ORDER BY seq
	`, s.limit(), session)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var events []core.Event
	var skipped []unreadable
	total := 0
	for rows.Next() {
		var seq uint64
		var size int
		var data []byte
		if err := rows.Scan(&seq, &size, &data); err != nil {
			return nil, nil, err
		}
		total += size
		if total > MaxScanBytes {
			return nil, nil, fmt.Errorf("conversation exceeds %d MiB query scan limit", MaxScanBytes>>20)
		}
		if size > s.limit() {
			skipped = append(skipped, unreadable{Seq: seq, Bytes: size})
			continue
		}
		var e core.Event
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, nil, fmt.Errorf("decode conversation event %d: %w", seq, err)
		}
		if e.Seq != seq || seq == 0 || (e.SessionID != "" && e.SessionID != session) {
			return nil, nil, fmt.Errorf("invalid stored event identity or sequence")
		}
		events = append(events, e)
	}
	return events, skipped, rows.Err()
}
