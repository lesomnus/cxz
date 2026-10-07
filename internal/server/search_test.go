package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/payday/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type searchCollector struct {
	grpc.ServerStream
	ctx      context.Context
	visits   []*api.SearchVisit
	progress []*api.SearchProgress
	summary  *api.SearchSummary
}

func (c *searchCollector) Context() context.Context { return c.ctx }
func (c *searchCollector) Send(r *api.SearchReply) error {
	switch {
	case r.Visit != nil:
		c.visits = append(c.visits, r.Visit)
	case r.Progress != nil:
		c.progress = append(c.progress, r.Progress)
	case r.Summary != nil:
		c.summary = r.Summary
	}
	return nil
}

// searchFixture is an installation whose events are stored but not yet
// extracted -- the state every installation is in the first time it is asked a
// question, because the index is derived and catching up is its own step.
func searchFixture(t *testing.T, sessions ...[]string) *Server {
	t.Helper()
	// A test process can itself be running inside a project container, where
	// asking for every conversation is refused.
	t.Setenv("CXZ_PROJECT_ID", "")
	ctx := t.Context()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: ":memory:", MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		"CREATE TABLE sessions(id TEXT PRIMARY KEY, manifest BLOB)",
		"CREATE TABLE events(session_id TEXT, seq INTEGER, data BLOB, PRIMARY KEY(session_id,seq))",
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{root: t.TempDir(), db: db}
	if err = s.openConversations(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.conversations.Close() })

	now := time.Now().UTC()
	for i, texts := range sessions {
		id := strings.Repeat(string(rune('a'+i)), 24)
		m := core.Session{ID: id, Title: "session " + id[:1], Kind: "claude", CreatedAt: now.Add(-90 * 24 * time.Hour).UnixMilli()}
		b, _ := json.Marshal(m)
		if _, err = db.Exec("INSERT INTO sessions VALUES(?,?)", id, b); err != nil {
			t.Fatal(err)
		}
		for j, text := range texts {
			// Older sessions first, and oldest first within one, which is the
			// order a journal is written in.
			age := time.Duration(len(sessions)-i)*24*time.Hour - time.Duration(j)*time.Hour
			kind := "input"
			if j%2 == 1 {
				kind = "assistant"
			}
			e := core.Event{SessionID: id, Seq: uint64(j + 1), Kind: kind, Text: text, TimeMS: now.Add(-age).UnixMilli()}
			data, _ := json.Marshal(e)
			if _, err = db.Exec("INSERT INTO events VALUES(?,?,?)", id, e.Seq, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	return s
}

// One query over the whole installation, newest conversation first.
func TestSearchAnswersFromTheIndex(t *testing.T) {
	s := searchFixture(t,
		[]string{"the relay refused the certificate", "nothing to do with it"},
		[]string{"why does the relay dial the alias", "because ssh resolves it"})
	c := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay"}, c); err != nil {
		t.Fatal(err)
	}
	if len(c.visits) != 2 {
		t.Fatalf("%d visits", len(c.visits))
	}
	if c.visits[0].SessionId != strings.Repeat("b", 24) {
		t.Fatal("the newest conversation is not first:", c.visits[0].SessionId)
	}
	hit := c.visits[0].Hits[0]
	if !strings.Contains(hit.Snippet, "relay") || hit.TimeMs == 0 || hit.Kind != "input" {
		t.Fatalf("%+v", hit)
	}
	if c.visits[0].Title != "session b" || c.visits[0].Agent != "claude" {
		t.Fatalf("%+v", c.visits[0])
	}
	if c.summary == nil || c.summary.Hits != 2 || c.summary.Sessions != 2 || c.summary.HasMore {
		t.Fatalf("%+v", c.summary)
	}
	// Stored events were extracted before the question was answered, and the
	// reply says what it caught up on.
	if len(c.progress) != 2 {
		t.Fatalf("%+v", c.progress)
	}
	// Asked again there is nothing to catch up on: the index is current.
	again := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay"}, again); err != nil {
		t.Fatal(err)
	}
	if len(again.progress) != 0 || again.summary.Hits != 2 {
		t.Fatalf("%+v %+v", again.progress, again.summary)
	}
	if again.summary.Examined == 0 {
		t.Fatal("what the answer cost is not reported")
	}
}

// A page that fills up says where to continue, and continuing neither repeats
// nor skips -- the mechanism behind reading a month at a time.
func TestSearchContinuesWhereItStopped(t *testing.T) {
	s := searchFixture(t, []string{"relay one", "relay two", "relay three"})
	var seen []uint64
	cursor := ""
	for page := 0; page < 5; page++ {
		c := &searchCollector{ctx: context.Background()}
		if err := s.Search(&api.SearchRequest{Query: "relay", Limit: 1, Cursor: cursor}, c); err != nil {
			t.Fatal(err)
		}
		for _, v := range c.visits {
			for _, h := range v.Hits {
				seen = append(seen, h.Seq)
			}
		}
		if !c.summary.HasMore {
			break
		}
		if cursor = c.summary.NextCursor; cursor == "" {
			t.Fatal("more to read but nowhere to continue from")
		}
	}
	if len(seen) != 3 {
		t.Fatalf("read %v", seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] >= seen[i-1] {
			t.Fatalf("paging repeated or reordered: %v", seen)
		}
	}
	c := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay", Cursor: "not-a-cursor"}, c); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}

// A window is half-open, so the same message cannot be found in two of them.
func TestSearchWindowIsHalfOpen(t *testing.T) {
	s := searchFixture(t, []string{"relay once"})
	c := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay"}, c); err != nil || len(c.visits) != 1 {
		t.Fatal(len(c.visits), err)
	}
	at := c.visits[0].Hits[0].TimeMs
	open := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay", SinceMs: at}, open); err != nil || len(open.visits) != 1 {
		t.Fatal("since is not inclusive", len(open.visits), err)
	}
	closed := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay", UntilMs: at}, closed); err != nil || len(closed.visits) != 0 {
		t.Fatal("until is not exclusive", len(closed.visits), err)
	}
}

// An agent inside a project reaches the same runtime on a local socket. It may
// already read its own project's journals, but every conversation on the host is
// the owner's to read, and this is the surface that cannot tell them apart.
func TestSearchIsRefusedFromInsideAProject(t *testing.T) {
	s := searchFixture(t, []string{"relay"})
	t.Setenv("CXZ_PROJECT_ID", "project")
	c := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay"}, c); status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
	if len(c.visits) != 0 {
		t.Fatal("a refused search still answered")
	}
	if err := s.Search(&api.SearchRequest{Query: "relay"}, &searchCollector{ctx: context.WithValue(context.Background(), managerAuthorityKey{}, true)}); err != nil {
		t.Fatal(err)
	}
}

// A bad expression is the caller's mistake, reported as one.
func TestSearchRefusesAnImpossibleQuery(t *testing.T) {
	s := searchFixture(t, []string{"relay"})
	for _, r := range []*api.SearchRequest{
		{Query: "(", Match: "regex"},
		{Query: "x", Match: "elsewhere"},
		{Query: "x", SinceMs: time.Now().Add(time.Hour).UnixMilli(), UntilMs: time.Now().UnixMilli()},
	} {
		if err := s.Search(r, &searchCollector{ctx: context.Background()}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("%+v: %v", r, err)
		}
	}
}

// The index is derived, so a purged conversation leaves nothing searchable.
func TestPurgeForgetsTheConversation(t *testing.T) {
	s := searchFixture(t, []string{"relay one"}, []string{"relay two"})
	c := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay"}, c); err != nil {
		t.Fatal(err)
	}
	if c.summary.Hits != 2 {
		t.Fatal(c.summary.Hits)
	}
	gone := strings.Repeat("a", 24)
	s.forgetConversation(t.Context(), gone)
	for _, q := range []string{"DELETE FROM events WHERE session_id=?", "DELETE FROM sessions WHERE id=?"} {
		if _, err := s.db.Exec(q, gone); err != nil {
			t.Fatal(err)
		}
	}
	after := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay"}, after); err != nil {
		t.Fatal(err)
	}
	if after.summary.Hits != 1 {
		t.Fatalf("a purged conversation is still searchable: %+v", after.summary)
	}
}

// Catching up is bounded: a search is not the place to rebuild an index, so
// what it could not reach is named rather than waited for.
func TestCatchUpReportsWhatItCouldNotReach(t *testing.T) {
	s := searchFixture(t, []string{"relay"})
	s.catchUpBudget = time.Nanosecond
	pending := s.catchUp(context.Background())
	if len(pending) != 1 || pending[0].State != "behind" {
		t.Fatalf("%+v", pending)
	}
	if seq, err := s.conversations.Cursor(context.Background(), strings.Repeat("a", 24)); err != nil || seq != 0 {
		t.Fatal("a cancelled catch-up claimed to have indexed something:", seq, err)
	}
}
