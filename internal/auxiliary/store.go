package auxiliary

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/lesomnus/payday/config"
	// This package cannot work without a SQLite engine, so it links one rather
	// than requiring whoever uses it to remember.
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

// The store is what a task leaves behind. The JSON beside it holds the rolling
// context and the task in flight -- working state, which is rewritten every
// turn -- while this keeps what ran: one row per task, and one summary per
// turn. Before it, Job was a single pointer in that file, so the next turn
// overwrote the last one and nothing could be asked about what had happened.
//
// It is the manager's own database, beside conversations.db, for the same
// reason that one is: the manager owns what it derives, and a derived store
// belongs next to the thing it is derived from.

// TaskHistory is how many tasks one session keeps. A task is small, and the
// reason to keep them is to be able to ask what ran and what it cost; the cap
// is here so a long-lived session cannot grow this without end.
const TaskHistory = 200

// SummaryHistory is how many turns of one session keep a summary. The
// transcript draws them beside the turns on screen, so this is how far back
// scrolling still finds one.
const SummaryHistory = 200

// SessionHistory is how many sessions this store keeps anything for, matching
// the cap on the context files beside it. The per-session caps above bound one
// session; this bounds how many of them there can be, and it cannot be done by
// the file sweep: those files are named by a hash of the session, so a swept
// name no longer says which session it was.
const SessionHistory = 256

var auxSchema = []string{
	`CREATE TABLE IF NOT EXISTS tasks (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		id TEXT NOT NULL UNIQUE,
		session TEXT NOT NULL,
		run TEXT NOT NULL,
		turn INTEGER NOT NULL,
		revision TEXT NOT NULL,
		state TEXT NOT NULL,
		message TEXT NOT NULL DEFAULT '',
		kinds TEXT NOT NULL DEFAULT '',
		usage TEXT NOT NULL DEFAULT '',
		updated_ms INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS tasks_recent ON tasks(session, seq DESC)`,
	`CREATE TABLE IF NOT EXISTS results (
		task TEXT NOT NULL,
		kind TEXT NOT NULL,
		text TEXT NOT NULL,
		PRIMARY KEY (task, kind)
	)`,
	// A summary is kept per turn rather than per task: one turn has one
	// summary, and asking for it again replaces what the last task said about
	// it instead of adding a second answer beside the same reply.
	`CREATE TABLE IF NOT EXISTS summaries (
		session TEXT NOT NULL,
		run TEXT NOT NULL,
		turn INTEGER NOT NULL,
		text TEXT NOT NULL,
		updated_ms INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (session, run, turn)
	)`,
	`CREATE INDEX IF NOT EXISTS summaries_turn ON summaries(session, turn)`,
}

type store struct{ db *sql.DB }

func openStore(ctx context.Context, root string) (*store, error) {
	dsn := (&url.URL{Scheme: "file", Path: filepath.Join(root, "aux.db")}).String() +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: dsn, MaxOpenConns: 2}).Open(ctx)
	if err != nil {
		return nil, err
	}
	for _, q := range auxSchema {
		if _, err = db.ExecContext(ctx, q); err != nil {
			db.Close()
			return nil, fmt.Errorf("aux store: %w", err)
		}
	}
	return &store{db: db}, nil
}

func (s *store) Close() error { return s.db.Close() }

// putTask records a task as it is now. The same task is written repeatedly as
// it runs, because a caller watching one wants the summary that arrived before
// the suggestion beside it was finished.
func (s *store) putTask(session string, j *Job, now int64) error {
	if j == nil || j.ID == "" {
		return nil
	}
	kinds := ""
	for _, k := range []string{"summary", "suggestion"} {
		if k == "summary" && !j.SummaryRequested || k == "suggestion" && !j.SuggestionRequested {
			continue
		}
		if kinds != "" {
			kinds += ","
		}
		kinds += k
	}
	usage := ""
	if len(j.Usage) > 0 {
		b, err := json.Marshal(j.Usage)
		if err != nil {
			return err
		}
		usage = string(b)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO tasks (id, session, run, turn, revision, state, message, kinds, usage, updated_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			state=excluded.state, message=excluded.message, kinds=excluded.kinds,
			usage=excluded.usage, updated_ms=excluded.updated_ms`,
		j.ID, session, j.Run, j.Turn, j.Revision, j.Status, j.Error, kinds, usage, now); err != nil {
		return err
	}
	for kind, text := range map[string]string{"summary": j.Summary, "suggestion": j.Suggestion} {
		if text == "" {
			// A result that was dropped -- a stale suggestion, say -- is removed
			// rather than left behind as an answer that no longer stands.
			if _, err = tx.Exec(`DELETE FROM results WHERE task = ? AND kind = ?`, j.ID, kind); err != nil {
				return err
			}
			continue
		}
		if _, err = tx.Exec(`INSERT INTO results (task, kind, text) VALUES (?,?,?)
			ON CONFLICT(task, kind) DO UPDATE SET text=excluded.text`, j.ID, kind, text); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`DELETE FROM results WHERE task IN (
			SELECT id FROM tasks WHERE session = ? AND seq <= (
				SELECT seq FROM tasks WHERE session = ? ORDER BY seq DESC LIMIT 1 OFFSET ?))`,
		session, session, TaskHistory); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM tasks WHERE session = ? AND seq <= (
			SELECT seq FROM tasks WHERE session = ? ORDER BY seq DESC LIMIT 1 OFFSET ?)`,
		session, session, TaskHistory); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM results WHERE task IN (SELECT id FROM tasks WHERE session NOT IN (
			SELECT session FROM tasks GROUP BY session ORDER BY MAX(updated_ms) DESC LIMIT ?))`,
		SessionHistory); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM tasks WHERE session NOT IN (
			SELECT session FROM tasks GROUP BY session ORDER BY MAX(updated_ms) DESC LIMIT ?)`,
		SessionHistory); err != nil {
		return err
	}
	return tx.Commit()
}

// tasks is what ran, newest first.
func (s *store) tasks(session string, limit int) ([]Job, error) {
	if limit <= 0 || limit > TaskHistory {
		limit = TaskHistory
	}
	rows, err := s.db.Query(`SELECT t.id, t.run, t.turn, t.revision, t.state, t.message, t.kinds, t.usage,
			COALESCE((SELECT text FROM results WHERE task = t.id AND kind = 'summary'), ''),
			COALESCE((SELECT text FROM results WHERE task = t.id AND kind = 'suggestion'), '')
		FROM tasks t WHERE t.session = ? ORDER BY t.seq DESC LIMIT ?`, session, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		var kinds, usage string
		if err = rows.Scan(&j.ID, &j.Run, &j.Turn, &j.Revision, &j.Status, &j.Error, &kinds, &usage, &j.Summary, &j.Suggestion); err != nil {
			return nil, err
		}
		j.Session = session
		for _, k := range splitKinds(kinds) {
			switch k {
			case "summary":
				j.SummaryRequested = true
			case "suggestion":
				j.SuggestionRequested = true
			}
		}
		if usage != "" {
			if err = json.Unmarshal([]byte(usage), &j.Usage); err != nil {
				return nil, err
			}
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// putSummary records one turn's summary, replacing whatever the last task said
// about that turn.
func (s *store) putSummary(session string, v Summary, now int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO summaries (session, run, turn, text, updated_ms) VALUES (?,?,?,?,?)
		ON CONFLICT(session, run, turn) DO UPDATE SET text=excluded.text, updated_ms=excluded.updated_ms`,
		session, v.Run, v.Turn, v.Text, now); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM summaries WHERE session = ? AND turn <= (
			SELECT turn FROM summaries WHERE session = ? ORDER BY turn DESC LIMIT 1 OFFSET ?)`,
		session, session, SummaryHistory); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM summaries WHERE session NOT IN (
			SELECT session FROM summaries GROUP BY session ORDER BY MAX(updated_ms) DESC LIMIT ?)`,
		SessionHistory); err != nil {
		return err
	}
	return tx.Commit()
}

// summaries are oldest first, which is the order a transcript reads them in.
func (s *store) summaries(session string, afterTurn uint64, limit int) ([]Summary, error) {
	if limit <= 0 || limit > SummaryHistory {
		limit = SummaryHistory
	}
	rows, err := s.db.Query(`SELECT run, turn, text FROM summaries
		WHERE session = ? AND turn > ? ORDER BY turn DESC LIMIT ?`, session, afterTurn, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var v Summary
		if err = rows.Scan(&v.Run, &v.Turn, &v.Text); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *store) forget(session string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM results WHERE task IN (SELECT id FROM tasks WHERE session = ?)`, session); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM tasks WHERE session = ?`, session); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM summaries WHERE session = ?`, session); err != nil {
		return err
	}
	return tx.Commit()
}

func splitKinds(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}
