package convindex

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

type corpus struct {
	index *Index
	now   time.Time
	seq   map[string]uint64
}

func newCorpus(t *testing.T) *corpus {
	t.Helper()
	index, err := Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { index.Close() })
	return &corpus{index: index, now: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), seq: map[string]uint64{}}
}

// say records one message, as the event pipeline would.
func (c *corpus) say(t *testing.T, project, session, kind, text string, ago time.Duration) uint64 {
	t.Helper()
	c.seq[session]++
	e := core.Event{SessionID: session, Seq: c.seq[session], Kind: kind, Text: text, TimeMS: c.now.Add(-ago).UnixMilli()}
	s := Session{ID: session, Project: project, Title: "session " + session, Agent: "claude", CreatedMS: c.now.Add(-90 * 24 * time.Hour).UnixMilli()}
	if err := c.index.Ingest(t.Context(), s, []core.Event{e}, 0, 0); err != nil {
		t.Fatal(err)
	}
	return e.Seq
}

func (c *corpus) search(t *testing.T, q Query) ([]Visit, Result) {
	t.Helper()
	visits, out, err := c.index.Search(t.Context(), q, c.now)
	if err != nil {
		t.Fatal(err)
	}
	return visits, out
}

// The order a person asked for, from one query over every project.
func TestSearchOrdersEveryProjectNewestFirst(t *testing.T) {
	c := newCorpus(t)
	c.say(t, "alpha", "aaa", "input", "the relay refused the certificate", 20*24*time.Hour)
	c.say(t, "alpha", "aaa", "assistant", "because the alias is not a hostname", 19*24*time.Hour)
	c.say(t, "beta", "bbb", "input", "why does the relay dial the alias", 2*time.Hour)
	c.say(t, "beta", "bbb", "assistant", "ssh resolves it and a TCP dial cannot", time.Hour)

	visits, out := c.search(t, Query{Query: "relay"})
	if len(visits) != 2 || out.Hits != 2 {
		t.Fatalf("%+v %+v", visits, out)
	}
	if visits[0].Session != "bbb" || visits[1].Session != "aaa" {
		t.Fatalf("out of order: %s then %s", visits[0].Session, visits[1].Session)
	}
	if visits[0].Project != "beta" || visits[0].Agent != "claude" || visits[0].Title != "session bbb" {
		t.Fatalf("%+v", visits[0])
	}
	if !strings.Contains(visits[0].Hits[0].Snippet, "relay") || visits[0].Hits[0].Bytes == 0 {
		t.Fatalf("%+v", visits[0].Hits[0])
	}
	// The index does not invent a window: an absent lower bound means all of
	// it, and the thirty-day default belongs to the client that asked.
	if !out.Since.IsZero() || !out.Until.Equal(c.now) {
		t.Fatalf("the window it answered for is not reported: %+v", out)
	}
}

// Hits of one conversation arrive together, newest first, under the session
// they were said in.
func TestSearchGroupsByConversation(t *testing.T) {
	c := newCorpus(t)
	c.say(t, "p", "aaa", "input", "relay one", 5*time.Hour)
	c.say(t, "p", "bbb", "input", "relay two", 4*time.Hour)
	c.say(t, "p", "aaa", "input", "relay three", 3*time.Hour)
	visits, out := c.search(t, Query{Query: "relay"})
	if len(visits) != 2 || out.Hits != 3 {
		t.Fatalf("%+v", out)
	}
	// aaa holds the newest match, so it comes first, with both of its hits.
	if visits[0].Session != "aaa" || len(visits[0].Hits) != 2 {
		t.Fatalf("%+v", visits)
	}
	if !visits[0].Hits[0].Time.After(visits[0].Hits[1].Time) {
		t.Fatal("hits within a conversation are not newest first")
	}
	if visits[1].Session != "bbb" || len(visits[1].Hits) != 1 {
		t.Fatalf("%+v", visits)
	}
}

// A window is half-open, so the windows a person walks back through partition
// the history rather than overlapping it.
func TestSearchWindowsPartitionHistory(t *testing.T) {
	c := newCorpus(t)
	c.say(t, "p", "aaa", "input", "deploy failed first", 70*24*time.Hour)
	c.say(t, "p", "aaa", "input", "deploy failed again", 40*24*time.Hour)
	c.say(t, "p", "aaa", "input", "deploy failed", 10*24*time.Hour)
	month := 30 * 24 * time.Hour
	seen := map[uint64]int{}
	for i := 0; i < 3; i++ {
		until := c.now.Add(-time.Duration(i) * month)
		visits, out := c.search(t, Query{Query: "deploy", Since: until.Add(-month), Until: until})
		if out.Hits != 1 {
			t.Fatalf("window %d: %d hits", i, out.Hits)
		}
		seen[visits[0].Hits[0].Seq]++
		if out.HasMore {
			t.Fatalf("window %d reported more: %+v", i, out)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("windows overlapped or missed: %v", seen)
	}
	// An instant is in the window that starts on it, and not in the one that
	// ends there.
	at := c.now.Add(-10 * 24 * time.Hour)
	if _, out := c.search(t, Query{Query: "deploy", Since: at}); out.Hits != 1 {
		t.Fatal("since is not inclusive")
	}
	if _, out := c.search(t, Query{Query: "deploy", Until: at}); out.Hits != 2 {
		t.Fatal("until is not exclusive")
	}
}

// A page bounded by hits continues below what it showed, never below what it
// read, and never repeats.
func TestSearchPagesWithoutRepeating(t *testing.T) {
	c := newCorpus(t)
	for i := 0; i < 7; i++ {
		c.say(t, "p", "aaa", "input", "hit number "+string(rune('a'+i)), time.Duration(7-i)*time.Hour)
	}
	q := Query{Query: "hit number", Limit: 3}
	var all []Hit
	for page := 0; page < 10; page++ {
		visits, out := c.search(t, q)
		for _, v := range visits {
			all = append(all, v.Hits...)
		}
		if !out.HasMore {
			break
		}
		if out.Next == nil {
			t.Fatal("more to read and nowhere to continue from")
		}
		q.Resume = *out.Next
	}
	if len(all) != 7 {
		t.Fatalf("%d hits over pages", len(all))
	}
	seen := map[uint64]bool{}
	for i, h := range all {
		if seen[h.Seq] {
			t.Fatalf("hit %d repeated sequence %d", i, h.Seq)
		}
		seen[h.Seq] = true
		if i > 0 && !h.Time.Before(all[i-1].Time) {
			t.Fatal("paging broke the order")
		}
	}
}

// The cursor carries the window, so a continued page asks what the first page
// asked even though events have arrived above it since.
func TestCursorCarriesTheWindow(t *testing.T) {
	c := newCorpus(t)
	since, until := c.now.Add(-48*time.Hour), c.now.Add(-24*time.Hour)
	encoded := EncodeCursor(since, until, Resume{TimeMS: 1, Session: "aaa", Seq: 2})
	gotSince, gotUntil, resume, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !gotSince.Equal(since) || !gotUntil.Equal(until) || resume.Session != "aaa" || resume.Seq != 2 {
		t.Fatalf("%v %v %+v", gotSince, gotUntil, resume)
	}
	if _, _, _, err = DecodeCursor("not-a-cursor"); err == nil {
		t.Fatal("a damaged cursor was accepted")
	}
}

func TestSearchMatchModesAndSelection(t *testing.T) {
	c := newCorpus(t)
	c.say(t, "mine", "aaa", "input", "Login FAILED for alice", 3*time.Hour)
	c.say(t, "mine", "aaa", "assistant", "the login failed because the token expired", 2*time.Hour)
	c.say(t, "other", "bbb", "input", "cxz web up", time.Hour)

	if _, out := c.search(t, Query{Query: "login failed"}); out.Hits != 1 {
		t.Fatal("substring is case sensitive by default:", out.Hits)
	}
	if _, out := c.search(t, Query{Query: "login failed", IgnoreCase: true}); out.Hits != 2 {
		t.Fatal(out.Hits)
	}
	if _, out := c.search(t, Query{Query: "login.*expired", Match: "regex", IgnoreCase: true}); out.Hits != 1 {
		t.Fatal(out.Hits)
	}
	visits, out := c.search(t, Query{Query: "cxzweb", Match: "fuzzy", IgnoreCase: true})
	if out.Hits != 1 || visits[0].Hits[0].Score <= 0 {
		t.Fatalf("%+v", visits)
	}
	// A project selection is exact, and excluding takes from what it selected.
	if _, out = c.search(t, Query{Query: "", Projects: []string{"other"}}); out.Hits != 1 {
		t.Fatal(out.Hits)
	}
	if _, out = c.search(t, Query{Query: "", Exclude: []string{"other"}}); out.Hits != 2 {
		t.Fatal(out.Hits)
	}
	if _, out = c.search(t, Query{Sessions: []string{"bbb"}}); out.Hits != 1 {
		t.Fatal(out.Hits)
	}
	// An empty query with a window is a valid question: what was said then.
	if _, out = c.search(t, Query{Since: c.now.Add(-150 * time.Minute)}); out.Hits != 2 {
		t.Fatal(out.Hits)
	}
	for _, bad := range []Query{{Match: "fuzzy"}, {Match: "elsewhere"}, {Query: "(", Match: "regex"}, {Limit: -1}, {Limit: MaxLimit + 1}, {Since: c.now, Until: c.now.Add(-time.Hour)}} {
		if _, _, err := c.index.Search(t.Context(), bad, c.now); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

// The index holds the conversation and not the machinery's record of it, which
// is the whole reason it is small enough to query.
func TestIngestKeepsOnlyTheConversation(t *testing.T) {
	c := newCorpus(t)
	events := []core.Event{
		{SessionID: "aaa", Seq: 1, Kind: "input", Text: "find the relay", TimeMS: c.now.Add(-time.Hour).UnixMilli()},
		{SessionID: "aaa", Seq: 2, Kind: "raw", Raw: []byte("relay in a vendor frame"), TimeMS: c.now.Add(-time.Hour).UnixMilli()},
		{SessionID: "aaa", Seq: 3, Kind: "usage", Text: "relay", TimeMS: c.now.Add(-time.Hour).UnixMilli()},
		{SessionID: "aaa", Seq: 4, Kind: "models", Text: "relay", TimeMS: c.now.Add(-time.Hour).UnixMilli()},
		{SessionID: "aaa", Seq: 5, Kind: "tool_call", Text: "Bash", Payload: json.RawMessage(`{"command":"grep relay"}`), TimeMS: c.now.Add(-time.Hour).UnixMilli()},
	}
	if err := c.index.Ingest(t.Context(), Session{ID: "aaa", Project: "p"}, events, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, out := c.search(t, Query{Query: "relay"}); out.Hits != 1 {
		t.Fatalf("the vendor stream and telemetry were indexed: %d hits", out.Hits)
	}
	// A tool call's subject is in its arguments, so the payload is searchable
	// when tools are asked for.
	visits, out := c.search(t, Query{Query: "grep relay", IncludeTools: true})
	if out.Hits != 1 || visits[0].Hits[0].Kind != "tool_call" {
		t.Fatalf("%+v", visits)
	}
}

// Ingestion is idempotent, because the same events arrive more than once.
func TestIngestIsIdempotentAndTracksItsExtent(t *testing.T) {
	c := newCorpus(t)
	events := []core.Event{
		{SessionID: "aaa", Seq: 1, Kind: "input", Text: "relay one", TimeMS: c.now.Add(-2 * time.Hour).UnixMilli()},
		{SessionID: "aaa", Seq: 2, Kind: "assistant", Text: "relay two", TimeMS: c.now.Add(-time.Hour).UnixMilli()},
	}
	for i := 0; i < 3; i++ {
		if err := c.index.Ingest(t.Context(), Session{ID: "aaa", Project: "p"}, events, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, out := c.search(t, Query{Query: "relay"}); out.Hits != 2 {
		t.Fatalf("re-ingestion duplicated rows: %d", out.Hits)
	}
	seq, err := c.index.Cursor(t.Context(), "aaa")
	if err != nil || seq != 2 {
		t.Fatal(seq, err)
	}
	// A batch that starts past the cursor leaves a gap, so ingestion does not
	// claim to have read what it skipped -- but it keeps the rows, so filling
	// the gap later costs only the gap.
	if err = c.index.Ingest(t.Context(), Session{ID: "aaa"}, []core.Event{
		{SessionID: "aaa", Seq: 9, Kind: "input", Text: "relay nine", TimeMS: c.now.Add(-time.Minute).UnixMilli()},
	}, 0, 0); err != nil {
		t.Fatal(err)
	}
	if seq, err = c.index.Cursor(t.Context(), "aaa"); err != nil || seq != 2 {
		t.Fatal("a gap advanced the cursor:", seq, err)
	}
	if err = c.index.Ingest(t.Context(), Session{ID: "aaa"}, []core.Event{
		{SessionID: "aaa", Seq: 3, Kind: "input", Text: "relay three", TimeMS: c.now.Add(-30 * time.Minute).UnixMilli()},
	}, 0, 0); err != nil {
		t.Fatal(err)
	}
	if seq, err = c.index.Cursor(t.Context(), "aaa"); err != nil || seq != 3 {
		t.Fatal("filling the gap did not advance the cursor:", seq, err)
	}
	if _, out := c.search(t, Query{Query: "relay"}); out.Hits != 4 {
		t.Fatalf("%d", out.Hits)
	}
}

// A trimmed journal says so beside its results: finding nothing in a session
// whose start was removed is not evidence that nothing was said.
func TestIngestRecordsTrimming(t *testing.T) {
	c := newCorpus(t)
	if err := c.index.Ingest(t.Context(), Session{ID: "aaa", Project: "p"}, []core.Event{
		{SessionID: "aaa", Seq: 1, Kind: core.HistoryTrimmedKind, Payload: json.RawMessage(`{"through":1}`), TimeMS: c.now.Add(-2 * time.Hour).UnixMilli()},
		{SessionID: "aaa", Seq: 2, Kind: "input", Text: "after the trim: relay", TimeMS: c.now.Add(-time.Hour).UnixMilli()},
	}, 0, 0); err != nil {
		t.Fatal(err)
	}
	visits, out := c.search(t, Query{Query: "relay"})
	if out.Hits != 1 || !visits[0].Trimmed || out.Trimmed != 1 {
		t.Fatalf("%+v %+v", visits, out)
	}
}

// A message longer than the index keeps is cut, and says it was.
func TestIngestCutsEnormousMessages(t *testing.T) {
	c := newCorpus(t)
	if err := c.index.Ingest(t.Context(), Session{ID: "aaa", Project: "p"}, []core.Event{
		{SessionID: "aaa", Seq: 1, Kind: "tool_result", Text: "relay " + strings.Repeat("x", MaxText), TimeMS: c.now.Add(-time.Hour).UnixMilli()},
	}, 0, 0); err != nil {
		t.Fatal(err)
	}
	visits, out := c.search(t, Query{Query: "relay", IncludeTools: true})
	if out.Hits != 1 || !visits[0].Hits[0].Cut || visits[0].Hits[0].Bytes != MaxText {
		t.Fatalf("%+v", visits[0].Hits[0])
	}
}

// A purged session leaves nothing behind, and neither does a removed project.
func TestForgetting(t *testing.T) {
	c := newCorpus(t)
	c.say(t, "p", "aaa", "input", "relay one", time.Hour)
	c.say(t, "q", "bbb", "input", "relay two", time.Hour)
	if err := c.index.Forget(t.Context(), "aaa"); err != nil {
		t.Fatal(err)
	}
	if _, out := c.search(t, Query{Query: "relay"}); out.Hits != 1 {
		t.Fatal(out.Hits)
	}
	if err := c.index.ForgetProject(t.Context(), "q"); err != nil {
		t.Fatal(err)
	}
	if _, out := c.search(t, Query{Query: "relay"}); out.Hits != 0 {
		t.Fatal(out.Hits)
	}
	if err := c.index.ForgetProject(t.Context(), ""); err == nil {
		t.Fatal("every project was forgotten at once")
	}
}
