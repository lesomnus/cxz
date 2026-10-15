package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionpurge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func purgeIn(id string, dry bool) *api.SessionPurgeInput {
	return &api.SessionPurgeInput{SessionId: id, DryRun: dry}
}

func rows(t *testing.T, s *Server, query, id string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPurgeDropsTheDerivedProjectionAndTheJournal(t *testing.T) {
	ctx := context.Background()
	t.Setenv("CXZ_PROJECT_ID", "")
	s, m, l := projectionFixture(t)
	if _, err := l.AppendBatch([]core.Event{{Kind: "assistant", RunID: "run", Text: "remember this"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.lockProjection(ctx, m); err != nil {
		t.Fatal(err)
	}
	s.projections[m.ID].mu.Unlock()
	if rows(t, s, "SELECT count(*) FROM events WHERE session_id=?", m.ID) == 0 {
		t.Fatal("fixture never projected an event")
	}
	out, err := s.PurgeSession(ctx, purgeIn(m.ID, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Targets) == 0 || out.Targets[len(out.Targets)-1].Kind != "journal" {
		t.Fatal("unexpected targets", out.Targets)
	}
	for _, query := range []string{"SELECT count(*) FROM events WHERE session_id=?", "SELECT count(*) FROM sessions WHERE id=?"} {
		if n := rows(t, s, query, m.ID); n != 0 {
			t.Fatal(query, "left", n)
		}
	}
	if s.projections[m.ID] != nil {
		t.Fatal("in-memory projection survived the purge")
	}
	if _, err = os.Lstat(core.Dir(s.root, m.ID)); !os.IsNotExist(err) {
		t.Fatal("journal survived", err)
	}
	// The session is gone, so a second purge has nothing to find and says so.
	if _, err = s.PurgeSession(ctx, purgeIn(m.ID, false)); status.Code(err) != codes.NotFound {
		t.Fatal("purging a purged session did not report it missing", err)
	}
}

func TestPurgeDryRunLeavesTheProjectionAndJournalAlone(t *testing.T) {
	ctx := context.Background()
	t.Setenv("CXZ_PROJECT_ID", "")
	s, m, l := projectionFixture(t)
	if _, err := l.AppendBatch([]core.Event{{Kind: "assistant", RunID: "run", Text: "still here"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.lockProjection(ctx, m); err != nil {
		t.Fatal(err)
	}
	s.projections[m.ID].mu.Unlock()
	out, err := s.PurgeSession(ctx, purgeIn(m.ID, true))
	if err != nil {
		t.Fatal(err)
	}
	var stake int64
	for _, t := range out.Targets {
		stake += t.Bytes
	}
	if !out.DryRun || stake == 0 {
		t.Fatal("dry run reported nothing at stake", out)
	}
	if rows(t, s, "SELECT count(*) FROM sessions WHERE id=?", m.ID) != 1 {
		t.Fatal("dry run deleted the manifest row")
	}
	if rows(t, s, "SELECT count(*) FROM events WHERE session_id=?", m.ID) == 0 {
		t.Fatal("dry run deleted projected events")
	}
	if _, err = os.Stat(filepath.Join(core.Dir(s.root, m.ID), "events.jsonl")); err != nil {
		t.Fatal("dry run deleted the journal", err)
	}
}

func TestPurgeSessionCreatedWithAWebUUID(t *testing.T) {
	ctx := context.Background()
	t.Setenv("CXZ_PROJECT_ID", "")
	s, m, l := projectionFixture(t)
	m.CreateID = "9b3bc6ba-0626-4a43-86fd-9f2601234567"
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE sessions SET manifest=? WHERE id=?", manifest, m.ID); err != nil {
		t.Fatal(err)
	}
	if err = core.WriteJSON(filepath.Join(core.Dir(s.root, m.ID), "session.json"), m); err != nil {
		t.Fatal(err)
	}
	if _, err = l.AppendBatch([]core.Event{{Kind: "assistant", RunID: "run", Text: "remember this"}}); err != nil {
		t.Fatal(err)
	}
	profile := accounts.SessionRoot(s.root, m.CreateID)
	neighbour := accounts.SessionRoot(s.root, "another-web-client-key")
	for _, dir := range []string{profile, neighbour} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "transcript.jsonl"), []byte("conversation"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := s.PurgeSession(ctx, purgeIn(m.ID, true))
	if err != nil {
		t.Fatal("UUID creation key refused during preview", err)
	}
	want := map[string]string{"profile": profile, "journal": core.Dir(s.root, m.ID)}
	if !plan.DryRun || len(plan.Targets) != len(want) {
		t.Fatal("preview did not include profile and journal", plan)
	}
	for _, target := range plan.Targets {
		if want[target.Kind] != target.Path {
			t.Fatal("preview named the wrong target", target)
		}
		if _, err = os.Stat(target.Path); err != nil {
			t.Fatal("preview deleted data", err)
		}
	}
	if rows(t, s, "SELECT count(*) FROM sessions WHERE id=?", m.ID) != 1 {
		t.Fatal("preview deleted the session projection")
	}
	done, err := s.PurgeSession(ctx, purgeIn(m.ID, false))
	if err != nil {
		t.Fatal("UUID creation key refused during purge", err)
	}
	if len(done.Targets) != len(plan.Targets) {
		t.Fatal("purge differed from preview", done)
	}
	for _, path := range want {
		if _, err = os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("purge left data behind", path, err)
		}
	}
	if rows(t, s, "SELECT count(*) FROM sessions WHERE id=?", m.ID) != 0 {
		t.Fatal("purge left the session projection behind")
	}
	if _, err = os.Stat(filepath.Join(neighbour, "transcript.jsonl")); err != nil {
		t.Fatal("purge affected another session", err)
	}
}

// An agent inside a project can read its own journal; letting it unlink one would
// hand it a delete with no undo, and would skip the uploads only the manager
// sees. The manager reaches the same runtime over a token-authenticated channel.
func TestPurgeRefusesToRunFromInsideAProject(t *testing.T) {
	ctx := context.Background()
	s, m, _ := projectionFixture(t)
	t.Setenv("CXZ_PROJECT_ID", "project")
	if _, err := s.PurgeSession(ctx, purgeIn(m.ID, false)); status.Code(err) != codes.PermissionDenied {
		t.Fatal("a call on the in-container socket was allowed to purge", err)
	}
	if rows(t, s, "SELECT count(*) FROM sessions WHERE id=?", m.ID) != 1 {
		t.Fatal("a refused purge still deleted the manifest row")
	}
	if _, err := os.Stat(core.Dir(s.root, m.ID)); err != nil {
		t.Fatal("a refused purge still deleted the journal", err)
	}
	authorized := context.WithValue(ctx, managerAuthorityKey{}, true)
	if _, err := s.PurgeSession(authorized, purgeIn(m.ID, false)); err != nil {
		t.Fatal("the manager's own channel was refused", err)
	}
}

// A handle that is not a session id is refused before it can be joined onto a
// path. The envelope used to make this a question about JSON; now the only way
// to get it wrong is the handle itself.
func TestPurgeRefusesAMalformedRequest(t *testing.T) {
	ctx := context.Background()
	t.Setenv("CXZ_PROJECT_ID", "")
	s, _, _ := projectionFixture(t)
	for _, id := range []string{"", "../../etc", "not-hex", strings.Repeat("a", sessionpurge.MaxSpec)} {
		if _, err := s.PurgeSession(ctx, purgeIn(id, false)); err == nil {
			t.Fatal("accepted", id)
		}
	}
}
