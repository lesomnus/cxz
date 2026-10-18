package transcripthistory

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/api"
)

// A caller that serializes a session keeps the reduced state, so a change costs
// the records that changed. Without the cache every change rebuilds that state
// from the table. See docs/plans/incremental-projection.md.
func TestSyncReusesTheReducedStateAcrossCalls(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	var c Cache
	write(event(1, "input", "", ""), event(2, "assistant", "", ""))
	if err := SyncWith(ctx, db, "s", decode, &c); err != nil {
		t.Fatal(err)
	}
	at := restored.Load()
	for i := uint64(3); i < 8; i++ {
		write(event(i, "assistant", "", ""))
		if err := SyncWith(ctx, db, "s", decode, &c); err != nil {
			t.Fatal(err)
		}
	}
	if restored.Load() != at {
		t.Fatal("a warm cache still rebuilt the reduced state", restored.Load()-at, "times")
	}

	// The same five changes without a cache: each one rebuilds.
	cold, coldWrite := fixture(t)
	coldWrite(event(1, "input", "", ""), event(2, "assistant", "", ""))
	if err := Sync(ctx, cold, "s", decode); err != nil {
		t.Fatal(err)
	}
	at = restored.Load()
	for i := uint64(3); i < 8; i++ {
		coldWrite(event(i, "assistant", "", ""))
		if err := Sync(ctx, cold, "s", decode); err != nil {
			t.Fatal(err)
		}
	}
	if got := restored.Load() - at; got != 5 {
		t.Fatal("expected one rebuild per change without a cache, got", got)
	}

	// And the two paths agree on what the conversation is.
	warm, err := Tail(ctx, db, "s", 0, 4096, decode)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Tail(ctx, cold, "s", 0, 4096, decode)
	if err != nil {
		t.Fatal(err)
	}
	if len(warm) != len(plain) {
		t.Fatal("cached and uncached projections disagree", len(warm), len(plain))
	}
	for i := range warm {
		if warm[i].GetEvent().GetSeq() != plain[i].GetEvent().GetSeq() || warm[i].GetUpdated() != plain[i].GetUpdated() {
			t.Fatal("row differs at", i, warm[i], plain[i])
		}
	}
}

// A cache is only usable for the state it was built from. A trim, a rebuild, or
// another caller advancing the same table must send it back to the table.
func TestSyncDropsACacheThatNoLongerMatchesTheTable(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	var c Cache
	write(event(10, "input", "", ""), event(20, "assistant", "", ""))
	if err := SyncWith(ctx, db, "s", decode, &c); err != nil {
		t.Fatal(err)
	}

	// Someone else advances the table with no cache of their own.
	write(event(30, "assistant", "", ""))
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	// Nothing has been written since, so this look is the cheap one and the
	// cache is neither used nor needed.
	at := restored.Load()
	if err := SyncWith(ctx, db, "s", decode, &c); err != nil {
		t.Fatal(err)
	}
	if restored.Load() != at {
		t.Fatal("an unchanged table rebuilt the reduced state")
	}
	// The next real change does need it, and what the cache holds was built
	// from a state the table has left behind.
	write(event(40, "assistant", "", ""))
	if err := SyncWith(ctx, db, "s", decode, &c); err != nil {
		t.Fatal(err)
	}
	if restored.Load() != at+1 {
		t.Fatal("a stale cache was used instead of restored")
	}

	// Trimming moves the first endpoint, which is a rebuild: the cache must not
	// carry the trimmed rows into it.
	if _, err := db.Exec("DELETE FROM events WHERE session_id='s' AND seq<=20"); err != nil {
		t.Fatal(err)
	}
	if err := SyncWith(ctx, db, "s", decode, &c); err != nil {
		t.Fatal(err)
	}
	rows, err := Tail(ctx, db, "s", 0, 4096, decode)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.GetEvent().GetSeq() <= 20 {
			t.Fatal("a trimmed record survived in the cached projection", r)
		}
	}
	if len(rows) != 2 {
		t.Fatal("expected the surviving records", len(rows))
	}
}

// Holding the reduced state must cost what rebuilding it would have read, not
// everything applied since the process started.
func TestACachedProjectionStaysTheSizeOfTheOpenTurn(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	var c Cache
	seq := uint64(0)
	for i := 0; i < 20; i++ {
		input := event(seq+1, "input", "", "")
		assistant := event(seq+2, "assistant", "", `{"item":{"phase":"final_answer"}}`)
		assistant.Response = &api.ResponseMetadata{Model: "model", Phase: "final_answer"}
		end := event(seq+3, "turn_end", "", `{"duration_ms":10}`)
		end.Text = "completed"
		write(input, assistant, end)
		seq += 3
		if err := SyncWith(ctx, db, "s", decode, &c); err != nil {
			t.Fatal(err)
		}
	}
	if c.projection == nil {
		t.Fatal("twenty appends dropped the cache")
	}
	if n := len(c.projection.Rows); n > 3 {
		t.Fatal("the cache grew with the conversation:", n, "rows after 20 turns")
	}
	// And the conversation itself is all there.
	rows, err := Tail(ctx, db, "s", 0, 4096, decode)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 40 {
		t.Fatal("pruning the cache lost rows from the table", len(rows))
	}
}
