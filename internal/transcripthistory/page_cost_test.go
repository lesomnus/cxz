package transcripthistory

import (
	"context"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
)

// A page carries the newest records of a few metadata kinds beside its rows, and
// what it costs to find them must not be what the whole conversation wrote in
// those kinds. It used to be: ranking a join of every metadata record to its
// body put `events.data` in the window function's input, so a page carrying
// 0.2MB of metadata read the 18-28MB of native payloads behind 16-26k records
// and threw all but 32 per kind away -- 0.3 to 2.0s on a real installation,
// paid by every page including the single-row reads that open a conversation.
//
// The plan is the assertion because the cost is not in what the query answers,
// which did not change, but in what it reads to answer. A body may only be read
// once a record has survived its kind's cap, and the cap is applied by walking
// the tail of the record index -- so `events` is reachable only as a primary-key
// lookup, and never from inside the step that applies the cap. See cxz#227.
func TestAPageDoesNotReadTheMetadataBodiesItDiscards(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	write(event(1, "input", "", ""))
	for i := 0; i < metadataPerKind*4; i++ {
		write(event(uint64(10+i), "usage", "", `{"input_tokens":1}`))
	}
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	query, args := metadataQuery("s", 1_000_000)
	rows, err := db.QueryContext(ctx, `EXPLAIN QUERY PLAN `+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type step struct {
		id, parent int
		detail     string
	}
	var plan []step
	for rows.Next() {
		var s step
		var unused int
		if err = rows.Scan(&s.id, &s.parent, &unused, &s.detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, s)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 {
		t.Fatal("the metadata read has no plan")
	}
	capped := map[int]bool{}
	for _, s := range plan {
		if strings.Contains(s.detail, "transcript_records") {
			// The step that applies the cap, and anything nested under it.
			capped[s.id] = true
		}
		if capped[s.parent] {
			capped[s.id] = true
		}
	}
	for _, s := range plan {
		if !strings.Contains(s.detail, "events") {
			continue
		}
		if capped[s.parent] {
			t.Fatal("a metadata body is read while the cap is still being applied:", s.detail)
		}
		if !strings.Contains(s.detail, "seq=?") {
			t.Fatal("a metadata body is reached by something other than one lookup:", s.detail)
		}
	}
	for _, s := range plan {
		if strings.Contains(s.detail, "transcript_records") && !strings.Contains(s.detail, "transcript_metadata") {
			t.Fatal("the cap is applied without the index that makes it a short walk:", s.detail)
		}
	}
}

// The join this replaced dropped a record whose body was gone before it could
// take a slot under the cap; the lookup that replaced it answers with nothing
// for that record, and a page must skip it rather than fail to decode it.
func TestAPageSkipsAMetadataRecordWhoseBodyIsGone(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	write(event(1, "input", "", ""), event(2, "usage", "", `{"input_tokens":1}`), event(3, "usage", "", `{"input_tokens":2}`))
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	// Delete the body without rebuilding the projection, which is what a trim
	// looks like for the moment between the delete and the next sync.
	if _, err := db.Exec(`DELETE FROM events WHERE session_id='s' AND seq=2`); err != nil {
		t.Fatal(err)
	}
	page, err := Page(ctx, db, &api.TranscriptRequest{SessionId: "s", SnapshotSeq: 1_000_000}, decode)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range page.Metadata {
		if e.Seq == 2 {
			t.Fatal("a record with no body was carried")
		}
	}
	var kept []uint64
	for _, e := range page.Metadata {
		kept = append(kept, e.Seq)
	}
	if len(kept) != 1 || kept[0] != 3 {
		t.Fatal("the surviving metadata is not the record that still has a body", kept)
	}
}
