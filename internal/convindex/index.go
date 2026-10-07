// Package convindex keeps the conversation as something that can be asked a
// question, instead of recovering it from the log every time.
//
// The measurement that decides the design: in this repository's own journals,
// 275 MiB of events held 0.35 MiB of conversation text -- 0.13%. The rest is
// the machinery's own record, mostly the verbatim vendor stream. Searching by
// reading journals therefore read eight hundred times more than it examined,
// could not order sessions without reading the end of each file, could not
// bound a record's size, and deleted conversation to stay inside a byte budget
// that telemetry had spent.
//
// So the conversation is extracted once, as events are recorded, into a store
// that answers by query: ordering is ORDER BY, a window is WHERE, and paging is
// a cursor comparison. What a person searches is small enough that this needs
// no text index at all -- a month of conversation across every project is a few
// megabytes. An index earns its place a hundred times larger than that, and the
// schema here leaves room for one without changing anything that reads it.
//
// It is derived, never authoritative: the journals remain the record, every row
// here can be rebuilt from them, and what has not been ingested yet is reported
// rather than quietly missing.
package convindex

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/payday/config"
	// This package cannot work without a SQLite engine, so it links one rather
	// than requiring whoever uses it to remember.
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

const (
	// MaxText bounds one message. A tool result can be tens of megabytes, and a
	// person searching for a phrase does not need all of it; what is cut is
	// recorded so a miss in a huge output is explainable.
	MaxText = 256 << 10
	// DefaultLimit is a page of hits, which is also the memory one query costs.
	DefaultLimit = 200
	MaxLimit     = 2000
	// ScanBudget bounds a page the store cannot filter for itself: regex and
	// fuzzy are applied here, to rows, so a page reports how many it read.
	ScanBudget = 50000
)

// Index is the conversation of one installation: every project it owns, every
// session, every message that was said rather than recorded.
type Index struct {
	db *sql.DB
}

// Open creates or opens the index beside the state it describes. It is a file
// of its own rather than a table in cxz.db, for two reasons: the daemon's
// database runs on a single connection that a search would then queue behind,
// and a text index added here later must not be a module cxz.db's connections
// are required to understand.
func Open(ctx context.Context, root string) (*Index, error) {
	dsn := (&url.URL{Scheme: "file", Path: filepath.Join(root, "conversations.db")}).String() +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: dsn, MaxOpenConns: 4}).Open(ctx)
	if err != nil {
		return nil, err
	}
	for _, q := range schema {
		if _, err = db.ExecContext(ctx, q); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Index{db: db}, nil
}

func (i *Index) Close() error { return i.db.Close() }

var schema = []string{
	`CREATE TABLE IF NOT EXISTS sessions(
		session    TEXT PRIMARY KEY,
		project    TEXT NOT NULL DEFAULT '',
		title      TEXT NOT NULL DEFAULT '',
		agent      TEXT NOT NULL DEFAULT '',
		created_ms INTEGER NOT NULL DEFAULT 0,
		first_ms   INTEGER NOT NULL DEFAULT 0,
		last_ms    INTEGER NOT NULL DEFAULT 0,
		-- Events through here are ingested, with no gap behind them. A gap is
		-- why this is not simply the highest sequence seen: live events can
		-- arrive before a backfill has filled what came before.
		cursor_seq INTEGER NOT NULL DEFAULT 0,
		-- The journal says its own start was removed by the size limit, so an
		-- absence of results here is not evidence that nothing was said.
		trimmed    INTEGER NOT NULL DEFAULT 0,
		indexed_ms INTEGER NOT NULL DEFAULT 0)`,
	`CREATE TABLE IF NOT EXISTS messages(
		session TEXT NOT NULL,
		seq     INTEGER NOT NULL,
		time_ms INTEGER NOT NULL,
		kind    TEXT NOT NULL,
		text    TEXT NOT NULL,
		cut     INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY(session, seq))`,
	// The order every query reads in, which is the order a person asked for.
	`CREATE INDEX IF NOT EXISTS messages_recent ON messages(time_ms DESC, session DESC, seq DESC)`,
}

// Session is what the index knows about a conversation apart from its text.
type Session struct {
	ID        string
	Project   string
	Title     string
	Agent     string
	CreatedMS int64
}

// Kinds are the events that are the conversation. The vendor's own stream and
// the machinery's telemetry are not: they are the 99.87% this package exists to
// stop reading.
var Kinds = []string{"input", "assistant", "tool_call", "tool_output", "tool_result"}

func indexed(kind string) bool {
	for _, v := range Kinds {
		if v == kind {
			return true
		}
	}
	return false
}

// Text is what a query is matched against, which includes the structured
// payload: a tool call's subject is usually in its arguments.
func Text(e core.Event) (string, bool) {
	s := e.Text
	if len(e.Payload) > 0 {
		if s != "" {
			s += "\n"
		}
		s += string(e.Payload)
	}
	if len(s) > MaxText {
		return s[:MaxText], true
	}
	return s, false
}

// Ingest records what a batch of one session's events said, and that every
// sequence in [from, through] was considered. The span is explicit because most
// of a journal is not conversation: the cursor has to advance over what was
// skipped, or every pass would read it again to skip it again. Zero means "take
// the span from the events".
//
// It is idempotent, because the same events arrive more than once: a client
// reading history and the live stream both pass through here, and catching up
// deliberately re-reads.
func (i *Index) Ingest(ctx context.Context, s Session, events []core.Event, from, through uint64) error {
	if s.ID == "" {
		return fmt.Errorf("a session id is required")
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO sessions(session, project, title, agent, created_ms) VALUES(?,?,?,?,?)
		ON CONFLICT(session) DO UPDATE SET
			project = CASE WHEN excluded.project != '' THEN excluded.project ELSE sessions.project END,
			title = CASE WHEN excluded.title != '' THEN excluded.title ELSE sessions.title END,
			agent = CASE WHEN excluded.agent != '' THEN excluded.agent ELSE sessions.agent END,
			created_ms = CASE WHEN excluded.created_ms != 0 THEN excluded.created_ms ELSE sessions.created_ms END`,
		s.ID, s.Project, s.Title, s.Agent, s.CreatedMS); err != nil {
		return err
	}
	var cursor, first, last uint64
	var trimmed int
	if err = tx.QueryRowContext(ctx, "SELECT cursor_seq, first_ms, last_ms, trimmed FROM sessions WHERE session=?", s.ID).
		Scan(&cursor, &first, &last, &trimmed); err != nil {
		return err
	}
	lowest, highest := from, through
	for _, e := range events {
		if e.Seq == 0 {
			continue
		}
		if lowest == 0 || e.Seq < lowest {
			lowest = e.Seq
		}
		highest = max(highest, e.Seq)
		if e.Kind == core.HistoryCheckpointKind || core.HistoryFloor(e.Kind, e.Payload) > 0 {
			trimmed = 1
		}
		if !indexed(e.Kind) || e.TimeMS == 0 {
			continue
		}
		text, cut := Text(e)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, "INSERT OR REPLACE INTO messages(session, seq, time_ms, kind, text, cut) VALUES(?,?,?,?,?,?)",
			s.ID, e.Seq, e.TimeMS, e.Kind, text, boolInt(cut)); err != nil {
			return err
		}
		if first == 0 || uint64(e.TimeMS) < first {
			first = uint64(e.TimeMS)
		}
		last = max(last, uint64(e.TimeMS))
	}
	// Only a batch that continues where ingestion stopped may advance it. One
	// that starts past the cursor leaves a gap behind it, and its rows are kept
	// so that filling the gap later costs only the gap.
	if lowest > 0 && lowest <= cursor+1 {
		cursor = max(cursor, highest)
	}
	_, err = tx.ExecContext(ctx, "UPDATE sessions SET cursor_seq=?, first_ms=?, last_ms=?, trimmed=?, indexed_ms=? WHERE session=?",
		cursor, first, last, trimmed, time.Now().UnixMilli(), s.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Forget removes a purged session. The index is derived, so this is not a
// record being destroyed -- it is a copy being kept honest.
func (i *Index) Forget(ctx context.Context, session string) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{"DELETE FROM messages WHERE session=?", "DELETE FROM sessions WHERE session=?"} {
		if _, err = tx.ExecContext(ctx, q, session); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ForgetProject removes a project's sessions, for a project that was removed.
func (i *Index) ForgetProject(ctx context.Context, project string) error {
	if project == "" {
		return fmt.Errorf("a project id is required")
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		"DELETE FROM messages WHERE session IN (SELECT session FROM sessions WHERE project=?)",
		"DELETE FROM sessions WHERE project=?",
	} {
		if _, err = tx.ExecContext(ctx, q, project); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Cursor is how far a session has been ingested, which is how a backfill knows
// what to ask for and how a reply knows what it could not see.
type Cursor struct {
	Session string
	Project string
	Seq     uint64
	LastMS  int64
}

// Cursors reports every session the index holds, newest conversation first.
func (i *Index) Cursors(ctx context.Context) ([]Cursor, error) {
	rows, err := i.db.QueryContext(ctx, "SELECT session, project, cursor_seq, last_ms FROM sessions ORDER BY last_ms DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cursor
	for rows.Next() {
		var c Cursor
		if err = rows.Scan(&c.Session, &c.Project, &c.Seq, &c.LastMS); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Cursor reports one session's ingested extent; zero when it is unknown here.
func (i *Index) Cursor(ctx context.Context, session string) (uint64, error) {
	var seq uint64
	err := i.db.QueryRowContext(ctx, "SELECT cursor_seq FROM sessions WHERE session=?", session).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return seq, err
}
