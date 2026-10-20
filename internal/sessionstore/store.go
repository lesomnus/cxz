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
	"net/url"
	"path/filepath"
	"sync"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/internal/transcripthistory"
	"github.com/lesomnus/payday/config"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

// Name is the file a session's conversation lives in, beside its journal.
const Name = "conversation.db"

// Schema is the shape of the file. Monotone, and a writer that does not know a
// higher one refuses rather than guesses.
const Schema = 1

// Compat is the lowest schema generation that may still write this file. It
// moves only when a change means something different rather than adding to it,
// because moving it is what stops an older build -- the one a rollback or
// `cxz use` puts back -- from recording a conversation at all.
const Compat = 1

// Durability says whether this file is the truth yet. It is not, while the
// journal is still written: a derived store that is lost is rebuilt from the
// journal, so it buys nothing by syncing on every commit, which is the same
// reason convindex runs on NORMAL. Stage 5 turns the journal off and this to
// FULL in one change.
const Durability = "NORMAL"

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
}

func (s *Store) DB() *sql.DB { return s.db }
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

var meta = []string{
	`CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	// The events a session recorded, in the shape the reading projection reads.
	`CREATE TABLE IF NOT EXISTS events(session_id TEXT NOT NULL, seq INTEGER NOT NULL, data BLOB NOT NULL, PRIMARY KEY(session_id,seq))`,
}

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
// `eventwire.FromCore` rewrites two kinds, and a store that kept the rewritten
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
	return nil
}

// Decode reads a stored record the way a reader sees it. Exported because a
// caller that pages this store directly through `transcripthistory` needs the
// same rules.
func Decode(b []byte) (*api.Event, error) {
	var e core.Event
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return eventwire.FromCore(e), nil
}
