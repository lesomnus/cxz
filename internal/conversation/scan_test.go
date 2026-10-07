package conversation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
)

type corpus struct {
	root string
	now  time.Time
}

func newCorpus(t *testing.T) *corpus {
	t.Helper()
	return &corpus{root: t.TempDir(), now: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}
}

// session writes one session's manifest and journal, with each event's time
// given as an age: a search is about when something was said.
func (c *corpus) session(t *testing.T, project, title string, events ...any) string {
	t.Helper()
	id := core.ID()
	dir := core.Dir(c.root, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := core.Session{ID: id, ProjectID: project, Title: title, Kind: "claude", CreatedAt: c.now.Add(-30 * 24 * time.Hour).UnixMilli()}
	if err := core.WriteJSON(filepath.Join(dir, "session.json"), manifest); err != nil {
		t.Fatal(err)
	}
	log, err := journal.Open(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	// A journal is written as things happen, so it is in time order whatever
	// order a test lists its events in.
	type record struct {
		kind, text string
		age        time.Duration
	}
	var records []record
	for i := 0; i < len(events); i += 3 {
		records = append(records, record{events[i].(string), events[i+1].(string), events[i+2].(time.Duration)})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].age > records[j].age })
	for _, r := range records {
		if _, err = log.Append(core.Event{SessionID: id, Kind: r.kind, Text: r.text, TimeMS: c.now.Add(-r.age).UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (c *corpus) scanner(project string) *Scanner {
	return &Scanner{Root: c.root, Project: project, Now: func() time.Time { return c.now }}
}

// flat is a hit with the session it came from, which the scan reports once per
// visit. Tests read better against the flattened list.
type flat struct {
	ScanSession
	ScanHit
}

func collect(t *testing.T, s *Scanner, q ScanQuery) ([]flat, ScanResult) {
	t.Helper()
	var hits []flat
	out, err := s.Scan(t.Context(), q, func(v ScanVisit) error {
		for _, h := range v.Hits {
			hits = append(hits, flat{v.ScanSession, h})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits, out
}

// Newest first, across sessions, without a sort: the scan visits sessions by
// their newest event and each session newest event first.
func TestScanEmitsNewestFirstAcrossSessions(t *testing.T) {
	c := newCorpus(t)
	c.session(t, "p", "old work",
		"input", "the relay refused the certificate", 20*24*time.Hour,
		"assistant", "the relay was not running", 19*24*time.Hour)
	c.session(t, "p", "today",
		"input", "why does the relay dial the alias", 2*time.Hour,
		"assistant", "because ssh resolves it", 90*time.Minute,
		"input", "relay again", time.Minute)

	hits, out := collect(t, c.scanner("p"), ScanQuery{Query: "relay"})
	if len(hits) != 4 || out.Sessions != 2 || out.Scanned != 2 {
		t.Fatalf("%d hits %+v", len(hits), out)
	}
	for i := 1; i < len(hits); i++ {
		if hits[i].Time.After(hits[i-1].Time) {
			t.Fatalf("hit %d is newer than the one before it: %+v", i, hits)
		}
	}
	if hits[0].Title != "today" || hits[len(hits)-1].Title != "old work" {
		t.Fatalf("sessions came out in the wrong order: %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "relay again") {
		t.Fatal("no snippet:", hits[0].Snippet)
	}
	if hits[0].Bytes == 0 || hits[0].Agent != "claude" {
		t.Fatalf("%+v", hits[0])
	}
}

// A month at a time, continued by a cursor: the second window is a third call
// with the same shape, and the two never overlap.
func TestScanWindowsPartitionTheHistory(t *testing.T) {
	c := newCorpus(t)
	c.session(t, "p", "a",
		"input", "deploy failed", 10*24*time.Hour,
		"input", "deploy failed again", 40*24*time.Hour,
		"input", "deploy failed first", 70*24*time.Hour)
	s := c.scanner("p")
	month := 30 * 24 * time.Hour

	seen := map[uint64]int{}
	for i := 0; i < 3; i++ {
		until := c.now.Add(-time.Duration(i) * month)
		hits, out := collect(t, s, ScanQuery{Query: "deploy", Since: until.Add(-month), Until: until})
		if len(hits) != 1 {
			t.Fatalf("window %d: %d hits", i, len(hits))
		}
		seen[hits[0].Seq]++
		if out.Next != nil {
			t.Fatalf("window %d reported more to read: %+v", i, out)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("windows overlapped or missed: %v", seen)
	}
	// A window before everything is cheap: no session is even opened.
	_, out := collect(t, s, ScanQuery{Query: "deploy", Until: c.now.Add(-200 * 24 * time.Hour)})
	if out.Sessions != 0 || out.Scanned != 0 {
		t.Fatalf("%+v", out)
	}
}

// A page is bounded, and the cursor it hands back continues below the last hit
// without repeating it.
func TestScanPagesWithoutRepeating(t *testing.T) {
	c := newCorpus(t)
	var events []any
	for i := 0; i < 7; i++ {
		events = append(events, "input", "hit number "+string(rune('a'+i)), time.Duration(7-i)*time.Hour)
	}
	c.session(t, "p", "many", events...)
	s := c.scanner("p")

	var all []flat
	q := ScanQuery{Query: "hit number", Limit: 3}
	for page := 0; page < 10; page++ {
		hits, out := collect(t, s, q)
		all = append(all, hits...)
		if out.Next == nil {
			break
		}
		if len(hits) != 3 {
			t.Fatalf("a page that reported more returned %d", len(hits))
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

// Another project's sessions are not this scanner's to read, even in the same
// root -- the manifest is checked, as it is for an agent.
func TestScanStaysInsideItsProject(t *testing.T) {
	c := newCorpus(t)
	c.session(t, "mine", "mine", "input", "shared secret word", time.Hour)
	c.session(t, "theirs", "theirs", "input", "shared secret word", time.Minute)
	hits, _ := collect(t, c.scanner("mine"), ScanQuery{Query: "shared"})
	if len(hits) != 1 || hits[0].Title != "mine" {
		t.Fatalf("%+v", hits)
	}
	// Without a project named, a root is searched whole: that is the host-local
	// installation, where there are no projects to separate.
	hits, _ = collect(t, c.scanner(""), ScanQuery{Query: "shared"})
	if len(hits) != 2 {
		t.Fatalf("%+v", hits)
	}
}

func TestScanMatchModes(t *testing.T) {
	c := newCorpus(t)
	c.session(t, "p", "modes",
		"input", "Login FAILED for alice", 3*time.Hour,
		"assistant", "the login failed because the token expired", 2*time.Hour,
		"input", "cxz web up", time.Hour)
	s := c.scanner("p")

	if hits, _ := collect(t, s, ScanQuery{Query: "login failed"}); len(hits) != 1 {
		t.Fatalf("substring is case sensitive by default: %+v", hits)
	}
	if hits, _ := collect(t, s, ScanQuery{Query: "login failed", IgnoreCase: true}); len(hits) != 2 {
		t.Fatalf("%+v", hits)
	}
	if hits, _ := collect(t, s, ScanQuery{Query: "login.*expired", Match: "regex", IgnoreCase: true}); len(hits) != 1 {
		t.Fatalf("%+v", hits)
	}
	if _, err := s.Scan(t.Context(), ScanQuery{Query: "(", Match: "regex"}, func(ScanVisit) error { return nil }); err == nil {
		t.Fatal("an invalid expression was accepted")
	}
	// Fuzzy is for a half remembered phrase, and scores what it finds.
	hits, _ := collect(t, s, ScanQuery{Query: "cxzweb", Match: "fuzzy", IgnoreCase: true})
	if len(hits) != 1 || hits[0].Score <= 0 {
		t.Fatalf("%+v", hits)
	}
	// An empty query with a window is a valid question: what was said then.
	hits, _ = collect(t, s, ScanQuery{Since: c.now.Add(-150 * time.Minute)})
	if len(hits) != 2 {
		t.Fatalf("%+v", hits)
	}
	for _, bad := range []ScanQuery{{Match: "fuzzy"}, {Match: "elsewhere"}, {View: "sideways"}, {Limit: -1}, {Limit: MaxScanHits + 1}, {Snippet: MaxSnippet + 1}, {Since: c.now, Until: c.now.Add(-time.Hour)}} {
		if _, err := s.Scan(t.Context(), bad, func(ScanVisit) error { return nil }); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

// Tools and raw vendor events are off unless asked for, exactly as they are in
// the per-session search.
func TestScanViewsAndTools(t *testing.T) {
	c := newCorpus(t)
	id := c.session(t, "p", "views", "input", "ordinary message about tokens", time.Hour)
	log, err := journal.Open(filepath.Join(core.Dir(c.root, id), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	if _, err = log.Append(core.Event{SessionID: id, Kind: "tool_call", Text: "Bash", Payload: json.RawMessage(`{"command":"grep tokens"}`), TimeMS: c.now.Add(-30 * time.Minute).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, err = log.Append(core.Event{SessionID: id, Kind: "raw", Raw: []byte(`{"type":"tokens"}`), TimeMS: c.now.Add(-20 * time.Minute).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	s := c.scanner("p")
	if hits, _ := collect(t, s, ScanQuery{Query: "tokens"}); len(hits) != 1 {
		t.Fatalf("tools or raw events leaked into the conversation view: %+v", hits)
	}
	// A tool call's subject is in its arguments, so the payload is searched too.
	hits, _ := collect(t, s, ScanQuery{Query: "grep tokens", IncludeTools: true})
	if len(hits) != 1 || hits[0].Kind != "tool_call" {
		t.Fatalf("%+v", hits)
	}
	if hits, _ = collect(t, s, ScanQuery{Query: "tokens", View: "raw"}); len(hits) != 1 || hits[0].Kind != "raw" {
		t.Fatalf("%+v", hits)
	}
}

// A trimmed session reports it. An empty result from a session whose start was
// discarded is not evidence that nothing was said.
func TestScanReportsTrimmedHistory(t *testing.T) {
	c := newCorpus(t)
	id := c.session(t, "p", "trimmed", "input", "after the trim: relay", time.Hour)
	log, err := journal.Open(filepath.Join(core.Dir(c.root, id), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	if _, err = log.Append(core.Event{SessionID: id, Kind: core.HistoryTrimmedKind, Payload: json.RawMessage(`{"through":1}`), TimeMS: c.now.Add(-50 * time.Minute).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	hits, out := collect(t, c.scanner("p"), ScanQuery{Query: "relay"})
	if len(hits) != 1 || !hits[0].Truncated || out.Truncated != 1 {
		t.Fatalf("%+v %+v", hits, out)
	}
}

// Ordering reads a journal's tail rather than scanning it, and that read is
// bounded: a session can hold more recent history than it will walk back
// through, and a journal records whatever an agent read, so single records are
// occasionally enormous. Past the bound it gives up -- and giving up has to mean
// "read this session anyway" rather than "skip it" or "fail the search".
func TestScanDoesNotLoseASessionBehindAHugeRecord(t *testing.T) {
	c := newCorpus(t)
	id := c.session(t, "p", "huge", "input", "the relay refused the certificate", 40*24*time.Hour)
	log, err := journal.Open(filepath.Join(core.Dir(c.root, id), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	// Recent history past what a tail read will walk back through, so the older
	// window's answer is out of its reach.
	for i := 0; i <= maxRecord/(1<<20); i++ {
		if _, err = log.Append(core.Event{SessionID: id, Kind: "input", Text: strings.Repeat("x", 1<<20), TimeMS: c.now.Add(-time.Duration(i) * time.Minute).UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	log.Close()

	s := c.scanner("p")
	index, err := s.Index(t.Context(), ScanQuery{Query: "relay", Until: c.now.Add(-30 * 24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 1 || !index[0].Approximate {
		t.Fatalf("%+v", index)
	}
	hits, out := collect(t, s, ScanQuery{Query: "relay", Since: c.now.Add(-50 * 24 * time.Hour), Until: c.now.Add(-30 * 24 * time.Hour)})
	if len(hits) != 1 || out.Scanned != 1 {
		t.Fatalf("a session behind a huge record was lost: %+v %+v", hits, out)
	}
	// The ordinary window still measures the tail exactly: the newest record is
	// the first thing the walk sees.
	index, err = s.Index(t.Context(), ScanQuery{Query: "relay"})
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 1 || index[0].Approximate {
		t.Fatalf("%+v", index)
	}
}

func TestSnippetCollapsesAndCentres(t *testing.T) {
	text := "alpha\n\n   beta gamma   delta\nepsilon"
	i := strings.Index(text, "gamma")
	got := snippet(text, i, i+5, 11)
	if !strings.Contains(got, "gamma") || strings.Contains(got, "\n") || strings.Contains(got, "  ") {
		t.Fatalf("%q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("a cut snippet does not say so: %q", got)
	}
	// A snippet never splits a rune.
	korean := "한글 대화 기록을 검색한다"
	for budget := 1; budget < len(korean); budget++ {
		if !utf8ValidString(snippet(korean, 7, 13, budget)) {
			t.Fatalf("budget %d split a rune", budget)
		}
	}
	if snippet("anything", 0, 3, 0) != "" {
		t.Fatal("a snippet was produced without a budget")
	}
}

func utf8ValidString(s string) bool { return json.Valid([]byte(`"` + s + `"`)) }
