// Package sessionstore is one session's conversation, in one file the session
// owns. The journal is still the truth while this is derived; see
// docs/plans/session-conversation-store.md for the stages and what each one
// moves.
//
// The tables are the ones `internal/transcripthistory` already builds, with the
// same names and the same columns, `session_id` included even though a file
// holds one session. That is deliberate: `Page`, `Tail` and `Details` then read
// this file unchanged, so the reading projection has one implementation rather
// than two during a migration. A store is, in shape, cxz.db with one session in
// it.
package sessionstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/eventview"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transcripthistory"
	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

// Name is the file a session's conversation lives in, beside its journal.
const Name = "conversation.db"

// Schema is the shape of the file. Monotone, and a writer that does not know a
// higher one refuses rather than guesses.
//
// Generation 2 is the same shape with a different meaning: the journal is no
// longer written, so this file is the record rather than a copy of one. Nothing
// in it had to change, which is why this moves with no DDL behind it.
const Schema = 2

// Compat is the lowest schema generation that may still write this file. It
// moves only when a change means something different rather than adding to it,
// because moving it is what stops an older build -- the one a rollback or
// `cxz use` puts back -- from recording a conversation at all.
//
// It moves to 2 with the journal. A generation 1 build would read the journal
// it finds beside this file, see the file ahead of it, and rebuild the file
// from the journal -- which erases every conversation recorded since the
// journal stopped. Refusing to record is the lesser loss, and it says so.
// See cxz#278.
const Compat = 2

// Durability says whether this file is the truth. It is, now that the journal
// is no longer written: there is nothing left to rebuild it from, so every
// commit syncs. It was NORMAL while the journal carried the one fsync and this
// file was derived.
const Durability = "FULL"

func Path(dir string) string { return filepath.Join(dir, Name) }

// Store is one session's file, open for writing by that session's supervisor or
// for reading by anyone else.
type Store struct {
	db      *sql.DB
	session string
	// mu serializes appends, because the reduction cache behind them is owned by
	// whoever serializes the session -- the same contract journal.Log keeps for
	// AppendBatch, for the same reason.
	mu    sync.Mutex
	cache transcripthistory.Cache
	// background is each run's background tasks, folded as they are appended
	// and kept in the meta table beside the records.
	background map[string]*agentview.BackgroundState
}

func (s *Store) DB() *sql.DB { return s.db }
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

var meta = append([]string{
	`CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	// The events a session recorded, in the shape the reading projection reads.
	`CREATE TABLE IF NOT EXISTS events(session_id TEXT NOT NULL, seq INTEGER NOT NULL, data BLOB NOT NULL, PRIMARY KEY(session_id,seq))`,
	// And the projection's own tables, so that a store is the whole shape as
	// soon as it exists: a reader of a conversation with no events yet would
	// otherwise find no tables to read.
}, transcripthistory.Schema()...)

// Open prepares the file and answers what it is. A reader that finds a schema
// it does not know still reads what it can: refusing to read would make a
// conversation recorded by a newer cxz invisible rather than merely newer.
// A writer refuses, which is what `write` decides.
func Open(ctx context.Context, path, session string, write bool) (*Store, error) {
	if session == "" {
		return nil, fmt.Errorf("session required")
	}
	query := "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(" + Durability + ")"
	if !write {
		query += "&mode=ro"
	}
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: (&url.URL{Scheme: "file", Path: path}).String() + query, MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, session: session}
	if !write {
		return s, nil
	}
	for _, q := range meta {
		if _, err = db.ExecContext(ctx, q); err != nil {
			db.Close()
			return nil, err
		}
	}
	var schema, compat int
	if schema, err = s.number(ctx, "schema"); err != nil {
		db.Close()
		return nil, err
	}
	if compat, err = s.number(ctx, "compat"); err != nil {
		db.Close()
		return nil, err
	}
	if compat > Schema {
		db.Close()
		return nil, fmt.Errorf("conversation %s was recorded by a newer cxz (compat %d, this build understands %d)", session, compat, Schema)
	}
	if schema == 0 {
		if err = s.stamp(ctx); err != nil {
			db.Close()
			return nil, err
		}
	} else if schema < Schema {
		// The owner migrates, once, when it opens the file to write. Only the
		// generation moves: the rows are already in the shape this build reads,
		// and rewriting `rows_format` here would claim a reduction this build
		// has not actually performed.
		if err = s.generation(ctx); err != nil {
			db.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) number(ctx context.Context, key string) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM meta WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}

// generation records that this file is now read and written by a newer cxz,
// and that an older one must not write it.
func (s *Store) generation(ctx context.Context) error {
	for key, value := range map[string]int{"schema": Schema, "compat": Compat} {
		if _, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES(?,?)`, key, fmt.Sprint(value)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) stamp(ctx context.Context) error {
	for key, value := range map[string]int{"schema": Schema, "compat": Compat, "rows_format": transcripthistory.Format()} {
		if _, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES(?,?)`, key, fmt.Sprint(value)); err != nil {
			return err
		}
	}
	return nil
}

// Schema reports the shape this file is in, and Compat the lowest generation
// that may write it. Both are zero in a file nothing has written yet.
func (s *Store) Schema(ctx context.Context) (int, error) { return s.number(ctx, "schema") }
func (s *Store) Compat(ctx context.Context) (int, error) { return s.number(ctx, "compat") }

// Append records events and the rows they reduce to, in one transaction. The
// reduction is the same code the server runs today, so a row written here and a
// row written there are the same bytes -- which is the property the parity test
// pins and the later read switch depends on.
//
// The records are stored as the journal stores them, not as a reader sees them:
// `eventview.FromCore` rewrites two kinds, and a store that kept the rewritten
// form could not answer what the provider actually said. The rewrite happens on
// the way out, here as everywhere.
//
// A reduction that cannot be computed must not cost the recording: the events
// commit regardless, and the rows are rebuilt on the next append or by the
// converter. Nothing a provider sends should be able to stop a conversation
// from being written down.
func (s *Store) Append(ctx context.Context, events []core.Event) error {
	if len(events) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range events {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO events VALUES(?,?,?)`, s.session, e.Seq, b); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// Sync reads the events table it was just given and writes the rows beside
	// them. It is incremental, so this costs the appended suffix rather than the
	// conversation.
	if err = transcripthistory.SyncWith(ctx, s.db, s.session, Decode, &s.cache); err != nil {
		return fmt.Errorf("reduce %s: %w", s.session, err)
	}
	return s.foldBackground(ctx, events)
}

// foldBackground keeps each run's background tasks beside the records.
//
// It is folded here, by the writer, because it cannot be found again
// afterwards: for one provider a background frame is an event of its own, and
// for the other it is a raw frame that only the reading view turns into one --
// and raw frames get no row in the reading projection, so a reader looking for
// them would have to walk the whole conversation. Reading state a writer
// already computed is the point of this file. See cxz#278.
func (s *Store) foldBackground(ctx context.Context, events []core.Event) error {
	changed := false
	for _, e := range events {
		v := eventview.FromCore(e)
		if v.Kind != "background" {
			continue
		}
		if s.background == nil {
			if err := s.loadBackground(ctx); err != nil {
				return err
			}
		}
		if s.background[e.RunID] == nil {
			s.background[e.RunID] = &agentview.BackgroundState{}
		}
		s.background[e.RunID].Apply(v.Payload)
		changed = true
	}
	if !changed {
		return nil
	}
	b, err := json.Marshal(s.background)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('background',?)`, string(b))
	return err
}

func (s *Store) loadBackground(ctx context.Context) error {
	s.background = map[string]*agentview.BackgroundState{}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='background'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), &s.background)
}

// Background is each run's background tasks as the session recorded them.
func (s *Store) Background(ctx context.Context) (map[string]*agentview.BackgroundState, error) {
	out := map[string]*agentview.BackgroundState{}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='background'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Decode reads a stored record the way a reader sees it. Exported because a
// caller that pages this store directly through `transcripthistory` needs the
// same rules.
func Decode(b []byte) (*api.Event, error) {
	var e core.Event
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return eventview.FromCore(e), nil
}

// Through is the highest sequence this store holds, or zero when it holds
// nothing. It is what tells a conversion whether it has anything left to do.
func (s *Store) Through(ctx context.Context) (uint64, error) {
	var v uint64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM events WHERE session_id=?`, s.session).Scan(&v)
	return v, err
}

// Fill records the part of a journal the store does not have yet, in batches so
// that it never holds the write lock for the whole of a long conversation. It
// answers how many records it wrote.
func (s *Store) Fill(ctx context.Context, events []core.Event, through uint64) (int, error) {
	pending := make([]core.Event, 0, len(events))
	for _, e := range events {
		if e.Seq > through {
			pending = append(pending, e)
		}
	}
	const batch = 2048
	for i := 0; i < len(pending); i += batch {
		if err := s.Append(ctx, pending[i:min(i+batch, len(pending))]); err != nil {
			return i, err
		}
	}
	return len(pending), nil
}

// Fence is what makes a recorded position trustworthy after a restart: the
// journal the position belongs to, as it looked when the position was written.
// Anything else and a reader goes back to reading the journal, which is what it
// did before this existed.
type state struct {
	Snapshot core.Snapshot `json:"snapshot"`
}

// SetState records the reduced session state beside the records it was reduced
// from. A reader has the state without replaying anything, which is the whole
// reason this store exists.
//
// It used to carry a fence -- the size and mtime of the journal it was reduced
// from -- because the journal was the record and this file could be behind it.
// There is no journal to be behind any more. See cxz#278.
func (s *Store) SetState(ctx context.Context, snap core.Snapshot) error {
	b, err := json.Marshal(state{Snapshot: snap})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('state',?)`, string(b))
	return err
}

// State answers the recorded state, and whether there is one.
func (s *Store) State(ctx context.Context) (core.Snapshot, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='state'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return core.Snapshot{}, false, nil
	}
	if err != nil {
		return core.Snapshot{}, false, err
	}
	var v state
	if err = json.Unmarshal([]byte(raw), &v); err != nil {
		return core.Snapshot{}, false, err
	}
	return v.Snapshot, true, nil
}

// LatestPerRun answers the newest record of a kind for each run, which is how a
// reduction that only cares about the last one -- a model catalog, republished
// whenever a setting changes -- is found without replaying a conversation. The
// record index is keyed by kind, so this reads the index and then the handful of
// bodies that survive it.
func (s *Store) LatestPerRun(ctx context.Context, kind string) ([]core.Event, error) {
	return s.records(ctx, `SELECT (SELECT data FROM events WHERE session_id=? AND seq=k.seq) FROM`+
		` (SELECT run_id, MAX(seq) AS seq FROM transcript_records WHERE session_id=? AND kind=? GROUP BY run_id) k`+
		` ORDER BY k.seq`, s.session, s.session, kind)
}

// EachOfKind answers every record of a kind in order, for a reduction that is a
// fold rather than a last-one-wins. It is bounded by how many records of that
// kind a session wrote, not by how long the conversation is.
func (s *Store) EachOfKind(ctx context.Context, kind string) ([]core.Event, error) {
	return s.records(ctx, `SELECT (SELECT data FROM events WHERE session_id=? AND seq=k.seq) FROM`+
		` (SELECT seq FROM transcript_records WHERE session_id=? AND kind=? ORDER BY seq) k`+
		` ORDER BY k.seq`, s.session, s.session, kind)
}

func (s *Store) records(ctx context.Context, query string, args ...any) ([]core.Event, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Event
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if b == nil {
			// A record whose body is gone, which a trim can leave behind until
			// the rows are rebuilt. It is not a record any more.
			continue
		}
		var e core.Event
		if err = json.Unmarshal(b, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Trim makes the store hold what the journal holds after a compaction: the
// records below the checkpoint are gone, and the checkpoint stands where they
// were. The rows are rebuilt rather than edited, because the first sequence
// moving is exactly what the reading projection treats as a rebuild -- a row
// reduced from records that no longer exist is not a row anyone should read.
func (s *Store) Trim(ctx context.Context, checkpoint core.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM events WHERE session_id=? AND seq<?`, s.session, checkpoint.Seq); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO events VALUES(?,?,?)`, s.session, checkpoint.Seq, b); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.cache.Drop()
	return transcripthistory.SyncWith(ctx, s.db, s.session, Decode, &s.cache)
}

// Events is the whole conversation in sequence order, which is how a session
// loads its own history at startup.
func (s *Store) Events(ctx context.Context) ([]core.Event, error) {
	return s.records(ctx, `SELECT data FROM events WHERE session_id=? ORDER BY seq`, s.session)
}

// Bytes is how much conversation is stored: the events of this session as
// recorded, which is the same count the journal's file size used to give and
// the same one a retention policy picks its cut against.
//
// It is deliberately not the file, and not the pages in use either. A delete
// does not return pages to the filesystem, so the file stays at its high-water
// mark -- but the pages in use do not come back down to the conversation
// either: they carry the index, the derived transcript projection, and the
// overflow pages of blobs that are still stored. On a 79MB conversation they
// read 143MB, so a policy with a 100MiB budget trimmed, measured no
// improvement, and trimmed again every turn. See cxz#286.
//
// SQLite answers length() on a blob out of the record header, so this does not
// read the conversation to measure it.
func (s *Store) Bytes(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(sum(length(data)),0) FROM events WHERE session_id=?`, s.session).Scan(&total)
	return total, err
}

// Replace rewrites events that are already stored, keeping their sequence. It
// is how a payload is shed: the record stays where it was and says what it was,
// with the verbatim bytes gone.
func (s *Store) Replace(ctx context.Context, events []core.Event) error {
	if len(events) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range events {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE events SET data=? WHERE session_id=? AND seq=?`, b, s.session, e.Seq); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Recent is the tail of a conversation: the last limit records, in order. It is
// what a diagnostic read wants -- the journal's tail, now that the tail is rows
// rather than the end of a file. See cxz#278.
func (s *Store) Recent(ctx context.Context, limit int) ([]core.Event, error) {
	if limit <= 0 {
		return nil, nil
	}
	out, err := s.records(ctx, `SELECT data FROM (SELECT seq, data FROM events WHERE session_id=?`+
		` ORDER BY seq DESC LIMIT ?) ORDER BY seq`, s.session, limit)
	return out, err
}
