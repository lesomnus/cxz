package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
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

// searchFixture is a host-local installation: one state directory, several
// sessions, no manager and no containers.
func searchFixture(t *testing.T, sessions ...[]string) *Server {
	t.Helper()
	// A test process can itself be running inside a project container, where
	// the refusal below would apply to every case.
	t.Setenv("CXZ_PROJECT_ID", "")
	s := &Server{root: t.TempDir()}
	now := time.Now().UTC()
	for i, texts := range sessions {
		id := strings.Repeat(string(rune('a'+i)), 24)
		dir := core.Dir(s.root, id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := core.WriteJSON(filepath.Join(dir, "session.json"), core.Session{ID: id, Title: "session " + id[:1], Kind: "claude"}); err != nil {
			t.Fatal(err)
		}
		log, err := journal.Open(filepath.Join(dir, "events.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		for j, text := range texts {
			// Older sessions first, and within a session oldest first, which is
			// how a journal is written.
			age := time.Duration(len(sessions)-i)*24*time.Hour - time.Duration(j)*time.Hour
			if _, err = log.Append(core.Event{SessionID: id, Kind: "input", Text: text, TimeMS: now.Add(-age).UnixMilli()}); err != nil {
				t.Fatal(err)
			}
		}
		log.Close()
	}
	return s
}

// The whole installation, newest first, in one stream.
func TestSearchReadsEverySessionNewestFirst(t *testing.T) {
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
}

// A page that fills up says where to continue, and continuing neither repeats
// nor skips -- which is the whole mechanism behind reading a month at a time.
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
	// A cursor is opaque, and a damaged one is refused rather than ignored.
	c := &searchCollector{ctx: context.Background()}
	if err := s.Search(&api.SearchRequest{Query: "relay", Cursor: "not-a-cursor"}, c); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}

// A window is half-open, so the same event cannot be found in two of them.
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
// already read its own journals, but reading every conversation on the host is
// the owner's operation, and this is the surface that cannot tell them apart.
func TestSearchIsRefusedFromInsideAProject(t *testing.T) {
	s := searchFixture(t, []string{"relay"})
	t.Setenv("CXZ_PROJECT_ID", "project")
	c := &searchCollector{ctx: context.Background()}
	err := s.Search(&api.SearchRequest{Query: "relay"}, c)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
	if len(c.visits) != 0 {
		t.Fatal("a refused search still answered")
	}
	// The manager's own channel is authorised, and searches as before.
	if err = s.Search(&api.SearchRequest{Query: "relay"}, &searchCollector{ctx: context.WithValue(context.Background(), managerAuthorityKey{}, true)}); err != nil {
		t.Fatal(err)
	}
}

// A bad expression is the caller's mistake, reported as one.
func TestSearchRefusesAnImpossibleQuery(t *testing.T) {
	s := searchFixture(t, []string{"relay"})
	for _, r := range []*api.SearchRequest{
		{Query: "(", Match: "regex"},
		{Query: "x", Match: "elsewhere"},
		{Query: "x", View: "sideways"},
		{Query: "x", SinceMs: time.Now().Add(time.Hour).UnixMilli(), UntilMs: time.Now().UnixMilli()},
	} {
		if err := s.Search(r, &searchCollector{ctx: context.Background()}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("%+v: %v", r, err)
		}
	}
}
