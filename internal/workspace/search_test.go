package workspace

import (
	"errors"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/conversation"
)

var errUnavailable = errors.New("retained storage unavailable")

// feed is one project's helper, scripted: an index first, then its sessions in
// the order the index promised.
type feed struct {
	reader *projectReader
	index  []conversation.ScanSession
	visits []conversation.ScanVisit
}

func session(id string, ago time.Duration, now time.Time) conversation.ScanSession {
	return conversation.ScanSession{ID: id, Activity: now.Add(-ago)}
}

func visitOf(s conversation.ScanSession, seqs ...uint64) conversation.ScanVisit {
	v := conversation.ScanVisit{ScanSession: s}
	for _, seq := range seqs {
		v.Hits = append(v.Hits, conversation.ScanHit{Seq: seq, Time: s.Activity})
	}
	return v
}

// mergeFixture runs the merge against scripted helpers. Messages are delivered
// in an order a real fan-out could produce -- every index first, then the
// visits interleaved -- which is what the merge has to cope with.
func mergeFixture(t *testing.T, limit int, feeds ...*feed) ([]SearchVisit, SearchResult) {
	t.Helper()
	messages := make(chan readerMessage, 256)
	var readers []*projectReader
	for _, f := range feeds {
		readers = append(readers, f.reader)
		messages <- readerMessage{reader: f.reader, message: conversation.ScanMessage{Index: f.index, Indexed: true}}
	}
	for i := 0; ; i++ {
		sent := false
		for _, f := range feeds {
			if i < len(f.visits) {
				messages <- readerMessage{reader: f.reader, message: conversation.ScanMessage{Visit: &f.visits[i]}}
				sent = true
			}
		}
		if !sent {
			break
		}
	}
	for _, f := range feeds {
		messages <- readerMessage{reader: f.reader, message: conversation.ScanMessage{Result: &conversation.ScanResult{Scanned: len(f.visits)}}}
		messages <- readerMessage{reader: f.reader, closed: true}
	}
	done := make(chan struct{})
	close(done)
	var got []SearchVisit
	out := SearchResult{}
	if err := merge(readers, messages, done, limit, func(v SearchVisit) error {
		got = append(got, v)
		return nil
	}, nil, func() {}, &out); err != nil {
		t.Fatal(err)
	}
	return got, out
}

// Conversations come out newest first whichever project they are in, which is
// the one thing a fan-out could easily get wrong.
func TestMergeOrdersSessionsAcrossProjects(t *testing.T) {
	now := time.Now().UTC()
	old := session("old", 30*24*time.Hour, now)
	middle := session("middle", 2*time.Hour, now)
	fresh := session("fresh", time.Minute, now)
	a := &feed{reader: &projectReader{project: &searchProject{id: "a", name: "alpha"}}, index: []conversation.ScanSession{middle, old}}
	a.visits = []conversation.ScanVisit{visitOf(middle, 9, 8), visitOf(old, 3)}
	b := &feed{reader: &projectReader{project: &searchProject{id: "b", name: "beta"}}, index: []conversation.ScanSession{fresh}}
	b.visits = []conversation.ScanVisit{visitOf(fresh, 5)}

	got, out := mergeFixture(t, 100, a, b)
	if len(got) != 3 {
		t.Fatalf("%d visits", len(got))
	}
	if got[0].ID != "fresh" || got[1].ID != "middle" || got[2].ID != "old" {
		t.Fatalf("out of order: %s %s %s", got[0].ID, got[1].ID, got[2].ID)
	}
	if got[0].ProjectID != "b" || got[1].ProjectName != "alpha" {
		t.Fatalf("%+v", got)
	}
	if out.Hits != 4 || out.Sessions != 3 {
		t.Fatalf("%+v", out)
	}
}

// A page stops at the limit, mid-session if that is where the limit falls, and
// remembers to continue below the hit it showed rather than the one the helper
// had read.
func TestMergeStopsAtTheLimitAndResumesInsideASession(t *testing.T) {
	now := time.Now().UTC()
	s := session("busy", time.Hour, now)
	f := &feed{reader: &projectReader{project: &searchProject{id: "a"}}, index: []conversation.ScanSession{s}}
	f.visits = []conversation.ScanVisit{visitOf(s, 9, 8, 7, 6)}

	got, out := mergeFixture(t, 2, f)
	if len(got) != 1 || len(got[0].Hits) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].Hits[1].Seq != 8 {
		t.Fatalf("%+v", got[0].Hits)
	}
	if out.Hits != 2 {
		t.Fatalf("%+v", out)
	}
	if f.reader.partial == nil || f.reader.partial.Seq != 8 || f.reader.partial.Session != "busy" {
		t.Fatalf("%+v", f.reader.partial)
	}
	// Which is what the cursor has to say, or the rest of that session is lost.
	cursor := conversation.ScanCursor{Version: 1}
	finish(&out, cursor, []*projectReader{f.reader}, conversation.ScanQuery{})
	if !out.HasMore || out.NextCursor == "" {
		t.Fatalf("%+v", out)
	}
	decoded, err := conversation.DecodeScanCursor(out.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.At["a"].Seq != 8 || decoded.Done["a"] {
		t.Fatalf("%+v", decoded)
	}
}

// A project that cannot be opened is reported and skipped. The search still
// answers for the others, and does not claim to have finished that one.
func TestMergeReportsAnUnreadableProject(t *testing.T) {
	now := time.Now().UTC()
	s := session("readable", time.Hour, now)
	good := &feed{reader: &projectReader{project: &searchProject{id: "good"}}, index: []conversation.ScanSession{s}}
	good.visits = []conversation.ScanVisit{visitOf(s, 1)}
	broken := &projectReader{project: &searchProject{id: "broken", name: "broken"}}

	messages := make(chan readerMessage, 16)
	messages <- readerMessage{reader: good.reader, message: conversation.ScanMessage{Index: good.index, Indexed: true}}
	messages <- readerMessage{reader: broken, failure: errUnavailable}
	messages <- readerMessage{reader: broken, closed: true}
	messages <- readerMessage{reader: good.reader, message: conversation.ScanMessage{Visit: &good.visits[0]}}
	messages <- readerMessage{reader: good.reader, message: conversation.ScanMessage{Result: &conversation.ScanResult{Scanned: 1}}}
	messages <- readerMessage{reader: good.reader, closed: true}
	done := make(chan struct{})
	close(done)

	var got []SearchVisit
	var reported []SearchProgress
	out := SearchResult{}
	if err := merge([]*projectReader{good.reader, broken}, messages, done, 10, func(v SearchVisit) error {
		got = append(got, v)
		return nil
	}, func(p SearchProgress) error {
		reported = append(reported, p)
		return nil
	}, func() {}, &out); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "readable" {
		t.Fatalf("%+v", got)
	}
	if out.Unavailable != 1 {
		t.Fatalf("%+v", out)
	}
	unavailable := false
	for _, p := range reported {
		if p.ProjectID == "broken" && p.State == "unavailable" && p.Message != "" {
			unavailable = true
		}
	}
	if !unavailable {
		t.Fatalf("the failure was not reported: %+v", reported)
	}
	cursor := conversation.ScanCursor{Version: 1}
	finish(&out, cursor, []*projectReader{good.reader, broken}, conversation.ScanQuery{})
	decoded, err := conversation.DecodeScanCursor(out.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Done["broken"] {
		t.Fatal("a project that was never read was marked finished")
	}
	if !decoded.Done["good"] {
		t.Fatal("a project that was read to the end will be read again")
	}
}

// The session's alias and state are the manager's to add; a helper has never
// heard of either.
func TestMergeDecoratesWithWhatOnlyTheManagerKnows(t *testing.T) {
	now := time.Now().UTC()
	s := session("abc", time.Hour, now)
	p := &searchProject{id: "a", name: "alpha", sessions: []*api.Session{{Id: "abc", Alias: "seal", State: "idle", Title: "the one about relays"}}}
	f := &feed{reader: &projectReader{project: p}, index: []conversation.ScanSession{s}}
	f.visits = []conversation.ScanVisit{visitOf(s, 1)}
	got, _ := mergeFixture(t, 10, f)
	if len(got) != 1 || got[0].Alias != "seal" || got[0].State != "idle" || got[0].Title != "the one about relays" {
		t.Fatalf("%+v", got)
	}
}
