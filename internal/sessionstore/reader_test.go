package sessionstore_test

import (
	"context"
	"os"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

// The manager reads a session's store from outside the container that writes
// it, which is what lets it stop keeping a copy (cxz#268). Both of the states
// it reads in have to work: a supervisor writing right now, and a session that
// has been stopped and the store closed.
func TestAReaderSeesALiveWriterAndAClosedStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := sessionstore.Path(dir)
	id := "ssssssssssssssssssssssss"

	w, err := sessionstore.Open(ctx, path, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Append(ctx, []core.Event{{SessionID: id, Seq: 1, Kind: "assistant", Text: "hi"}}); err != nil {
		t.Fatal(err)
	}

	count := func(when string) int {
		t.Helper()
		r, err := sessionstore.Open(ctx, path, id, false)
		if err != nil {
			t.Fatal("read-only open", when, err)
		}
		defer r.Close()
		var n int
		if err = r.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE session_id=?", id).Scan(&n); err != nil {
			t.Fatal("read", when, err)
		}
		return n
	}
	if n := count("beside a live writer"); n != 1 {
		t.Fatal("a reader did not see what the writer had committed:", n)
	}
	// Committed while a reader has already been and gone.
	if err = w.Append(ctx, []core.Event{{SessionID: id, Seq: 2, Kind: "assistant", Text: "more"}}); err != nil {
		t.Fatal(err)
	}
	if n := count("after a second commit"); n != 2 {
		t.Fatal("a reader saw a stale page:", n)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	// A closed store has no -wal to read through, which is the state a stopped
	// session's conversation is in.
	if _, err = os.Stat(path + "-wal"); !os.IsNotExist(err) {
		t.Log("-wal survived the close:", err)
	}
	if n := count("after the writer closed"); n != 2 {
		t.Fatal("a stopped session's conversation was not readable:", n)
	}
}
