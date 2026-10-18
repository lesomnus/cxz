package transcripthistory

import (
	"context"
	"testing"
)

// Rows polls Sync every 150ms for each open stream, so the cost of noticing
// that nothing happened must not grow with the conversation. See cxz#213.
func TestSyncWalksRecordsOncePerChangeNotOncePerLook(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	sync := func() {
		t.Helper()
		if err := Sync(ctx, db, "s", decode); err != nil {
			t.Fatal(err)
		}
	}
	walks := func() uint64 { return counted.Load() }

	write(event(10, "input", "", ""), event(30, "assistant", "", ""))
	start := walks()
	sync()
	if walks() == start {
		t.Fatal("a first projection did not walk the records")
	}
	at := walks()
	for i := 0; i < 5; i++ {
		sync()
	}
	if walks() != at {
		t.Fatal("an unchanged session was walked", walks()-at, "more times")
	}

	write(event(40, "assistant", "", ""))
	sync()
	if walks() != at+1 {
		t.Fatal("an append did not cost exactly one walk", walks()-at)
	}
	at = walks()
	sync()
	if walks() != at {
		t.Fatal("settling after an append walked again")
	}

	// A hole filled behind the cursor moves neither endpoint, which is why the
	// table's own high-water mark is part of the question.
	write(event(20, "assistant", "", ""))
	sync()
	if walks() != at+1 {
		t.Fatal("a filled hole did not cost one walk", walks()-at)
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
	// the first endpoint is what moves.
	at = walks()
	if _, err = db.Exec("DELETE FROM events WHERE session_id='s' AND seq<=20"); err != nil {
		t.Fatal(err)
	}
	sync()
	if walks() != at+1 {
		t.Fatal("a trimmed prefix did not cost one walk", walks()-at)
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
	at := counted.Load()
	if _, err := db.Exec("INSERT INTO events VALUES('other',1,'{}')"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := Sync(ctx, db, "s", decode); err != nil {
			t.Fatal(err)
		}
	}
	if got := counted.Load() - at; got != 1 {
		t.Fatal("expected one verification and then quiet, got", got)
	}
}
