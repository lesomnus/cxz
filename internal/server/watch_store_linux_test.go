package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/sessionstore"
)

// handles counts the descriptors this process holds on one path. It is the only
// way to see the thing cxz#314 was: a connection that is never closed is not
// visible in anything the server reports about itself, and on this driver each
// one carries a wasm sqlite rather than a file handle.
func handles(t *testing.T, path string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err != nil {
			continue // Closed while this list was being read.
		}
		if target == path {
			n++
		}
	}
	return n
}

// A record stream reads the conversation again on every write. It must read it
// through the connection it already has: one per read is what grew a runtime by
// 29MB a minute until the machine had nothing left. See cxz#314.
func TestRecordStreamKeepsOneConversationConnection(t *testing.T) {
	s, m, l := projectionFixture(t)
	path := sessionstore.Path(core.Dir(s.root, m.ID))
	write := func(text string) {
		t.Helper()
		if _, err := l.AppendBatch([]core.Event{{SessionID: m.ID, Kind: "input", Text: text, RunID: "run"}}); err != nil {
			t.Fatal(err)
		}
	}
	write("first")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const reads = 6
	var counts []int
	sink := &eventSink{ctx: ctx, cancel: cancel}
	sink.done = func(frames []*api.Event) bool {
		if frames[len(frames)-1].Heartbeat {
			return false
		}
		counts = append(counts, handles(t, path))
		if len(counts) >= reads {
			return true
		}
		// Another record, so the fence moves and the stream reads once more.
		// That read is where the connection was being opened.
		write(strconv.Itoa(len(counts)))
		return false
	}
	if err := s.Watch(&api.WatchRequest{SessionId: m.ID, ClientId: "test"}, sink); !errors.Is(err, context.Canceled) {
		t.Fatal("the stream ended for another reason", err)
	}
	if len(counts) < reads {
		t.Fatal("the stream did not read often enough to say anything", counts)
	}
	for _, n := range counts[1:] {
		if n != counts[0] {
			t.Fatal(fmt.Sprint("the stream opened a connection per read: ", counts))
		}
	}
	if counts[0] == 0 {
		t.Fatal("no connection to the conversation was held at all, so this proves nothing")
	}
}
