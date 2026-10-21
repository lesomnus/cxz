package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/assets"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventview"
	"github.com/lesomnus/cxz/internal/historypage"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// errNoConversation is a session with nothing written yet: a store is created
// by the supervisor at startup, so the window is small, and an empty page is a
// better answer than an error for a session that simply has no history.
var errNoConversation = errors.New("no conversation store")

// sessionProject is the project a session belongs to, from the inventory the
// manager already keeps.
func (m *Manager) sessionProject(ctx context.Context, session string) (*Project, error) {
	projects, err := m.all(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		for _, s := range p.Sessions {
			if s.Id == session {
				return p, nil
			}
		}
	}
	return nil, fmt.Errorf("session not found: %s", session)
}

// conversationStore opens a session's own conversation database, read-only,
// where its project keeps state inside the manager's volume.
//
// This is what moving project state into the manager's volume was for. The
// conversation is a file the manager can read, so it keeps no copy of its own
// and can still read it when the project is not running at all. Before this
// there were three mechanisms for the same file: every page was mirrored into
// the manager's `events` table, a project's whole history was paged out before
// its containers were removed, and that mirror was served whenever the project
// could not answer. See cxz#268.
// ConversationStore is conversationStore for the server that runs beside this
// manager: it reads the same files to seed the searchable index.
func (m *Manager) ConversationStore(ctx context.Context, session string) (*sessionstore.Store, error) {
	return m.conversationStore(ctx, session)
}

func (m *Manager) conversationStore(ctx context.Context, session string) (*sessionstore.Store, error) {
	if !assets.ValidID(session) {
		return nil, fmt.Errorf("invalid session id")
	}
	// An installation that still keeps project state in project volumes cannot
	// be read this way, and saying so is better than serving an empty
	// conversation: the project has to be recreated once to migrate it.
	if m.StateVolume == "" {
		return nil, fmt.Errorf("project state is not in the manager's volume; recreate the project to migrate it")
	}
	p, err := m.sessionProject(ctx, session)
	if err != nil {
		return nil, err
	}
	dir, _ := m.projectState(p.ID)
	sessionDir := core.Dir(filepath.Join(dir, "data"), session)
	path := sessionstore.Path(sessionDir)
	if _, err = os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// A session that has not run since stores existed has only its journal:
		// conversion happens when a supervisor starts, and a stopped session's
		// supervisor never will. Convert it here, once -- the manager is the
		// only thing that will ever look at that conversation again, and a
		// store for every session is also what lets the journal stop being
		// written at all. Re-runnable, so a conversion that is interrupted is
		// finished by the next read. See cxz#268.
		if _, err = os.Stat(filepath.Join(sessionDir, "events.jsonl")); err != nil {
			// No journal either: nothing was ever recorded for this session.
			return nil, errNoConversation
		}
		if _, err = sessionstore.Convert(ctx, sessionDir, session); err != nil {
			return nil, fmt.Errorf("convert session %s: %w", session, err)
		}
	}
	store, err := sessionstore.Open(ctx, path, session, false)
	if err != nil {
		return nil, err
	}
	// Opening is enough to create the `-wal` and `-shm` beside the file, and
	// the conversion created the file itself.
	if err = ownLikeDir(sessionDir, storeFiles(path)...); err != nil {
		store.Close()
		return nil, fmt.Errorf("hand session %s store back to its runtime: %w", session, err)
	}
	return store, nil
}

// Seams, so the thing that broke can be tested without being root.
var (
	effectiveUID = os.Geteuid
	handOver     = os.Lchown
)

// ownLikeDir gives files the manager created in a project's state to whoever
// owns the directory they are in.
//
// The manager runs as root and the project runtime does not, and SQLite opens
// `-shm` for writing even for a read-only connection -- so a store left owned
// by root is one the runtime cannot open at all. Every resume of a session the
// manager had converted failed with `invalid _pragma: unable to open database
// file`, 838 times in one runtime log, and the conversation was unreachable
// until the files were handed back by hand. See cxz#280.
func ownLikeDir(dir string, paths ...string) error {
	if effectiveUID() != 0 {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot read the owner of %s", dir)
	}
	for _, p := range paths {
		if err = handOver(p, int(owner.Uid), int(owner.Gid)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// storeFiles is the file a conversation lives in and the two SQLite keeps
// beside it. The `-shm` is the one that matters here: a reader needs to write
// it.
func storeFiles(path string) []string { return []string{path, path + "-wal", path + "-shm"} }

// storedHistory is one page of a session's conversation, read the way the
// project runtime reads its own: the rows are what the supervisor wrote.
func storedHistory(ctx context.Context, db *sql.DB, r *api.WatchRequest) (*api.EventBatch, error) {
	rows, err := db.QueryContext(ctx, "SELECT data FROM events WHERE session_id=? AND seq>? ORDER BY seq LIMIT ?", r.SessionId, r.AfterSeq, historypage.PageSize(r.Limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &api.EventBatch{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var v core.Event
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out.Events = append(out.Events, eventview.FromCore(v))
	}
	return out, rows.Err()
}
