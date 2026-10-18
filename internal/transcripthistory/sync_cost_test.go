package transcripthistory

import (
	"context"
	"testing"
)

// Rows polls Sync every 150ms for each open stream, so the cost of noticing
// that nothing happened must not grow with the conversation, and neither must
// the cost of noticing that something did. See cxz#213.
func TestSyncProbesOncePerChangeNotOncePerLook(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	sync := func() {
		t.Helper()
		if err := Sync(ctx, db, "s", decode); err != nil {
			t.Fatal(err)
		}
	}
	probes := func() uint64 { return probed.Load() }

	write(event(10, "input", "", ""), event(30, "assistant", "", ""))
	sync()
	// A first projection has no cursor to ask about; everything after it does.
	at := probes()
	for i := 0; i < 5; i++ {
		sync()
	}
	if probes() != at {
		t.Fatal("an unchanged session was probed", probes()-at, "more times")
	}

	write(event(40, "assistant", "", ""))
	sync()
	if probes() != at+1 {
		t.Fatal("an append did not cost exactly one probe", probes()-at)
	}
	at = probes()
	sync()
	if probes() != at {
		t.Fatal("settling after an append probed again")
	}

	// A hole filled behind the cursor moves neither endpoint, which is why the
	// table's own high-water mark is part of the question.
	write(event(20, "assistant", "", ""))
	sync()
	if probes() != at+1 {
		t.Fatal("a filled hole did not cost one probe", probes()-at)
	}
	rows, err := Tail(ctx, db, "s", 0, 64, decode)
	if err != nil {
		t.Fatal(err)
	}
	var seqs []uint64
	for _, r := range rows {
		seqs = append(seqs, r.GetEvent().GetSeq())
	}
	if len(seqs) != 4 || seqs[1] != 20 {
		t.Fatal("the filled hole is not in the projection", seqs)
	}

	// Trimming deletes without writing, so the high-water mark stands still and
	// the first endpoint is what moves -- which is already a rebuild, so there
	// is nothing to ask.
	at = probes()
	if _, err = db.Exec("DELETE FROM events WHERE session_id='s' AND seq<=20"); err != nil {
		t.Fatal(err)
	}
	sync()
	if probes() != at {
		t.Fatal("a trim that is already a rebuild still probed", probes()-at)
	}
	if rows, err = Tail(ctx, db, "s", 0, 64, decode); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if seq := r.GetEvent().GetSeq(); seq <= 20 {
			t.Fatal("a trimmed record survived in the projection", seq)
		}
	}
}

// Another session's write moves the table's high-water mark, so this session
// verifies once and then settles. It costs a walk it does not strictly need,
// which is the price of not having to trust every writer to announce itself.
func TestSyncSettlesAfterAnotherSessionWrites(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	write(event(1, "input", "", ""))
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	at := probed.Load()
	if _, err := db.Exec("INSERT INTO events VALUES('other',1,'{}')"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := Sync(ctx, db, "s", decode); err != nil {
			t.Fatal(err)
		}
	}
	if got := probed.Load() - at; got != 1 {
		t.Fatal("expected one verification and then quiet, got", got)
	}
}

// A projection made before there was a watermark must not be rebuilt just
// because it has none: the build that made it answered the same question by
// counting, so the mark is adopted and the sync stays incremental.
func TestAProjectionWithoutAWatermarkIsAdoptedNotRebuilt(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	write(event(1, "input", "", ""), event(2, "assistant", "", ""))
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM transcript_seen"); err != nil {
		t.Fatal(err)
	}
	write(event(3, "assistant", "", ""))
	probes, restores := probed.Load(), restored.Load()
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	if probed.Load() != probes {
		t.Fatal("probed without a watermark to probe against")
	}
	if restored.Load() != restores+1 {
		t.Fatal("an adopted projection was rebuilt instead of restored")
	}
	seqs := []uint64{}
	rows, err := Tail(ctx, db, "s", 0, 64, decode)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		seqs = append(seqs, r.GetEvent().GetSeq())
	}
	if len(seqs) != 3 {
		t.Fatal("the adopted projection lost records", seqs)
	}
	var seen uint64
	if err = db.QueryRow("SELECT written FROM transcript_seen WHERE session_id='s'").Scan(&seen); err != nil {
		t.Fatal("the watermark was not written back:", err)
	}
}
