package websandbox

import (
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/proto"
)

// The sandbox's stream and its page are the same projection, for the same
// reason the real one's are: a page a person scrolls back to must not differ
// from the row they watched arrive.
func TestSandboxRowsAgreeWithItsPage(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := s.Sessions()
	ref := resource.SessionRef_builder{RuntimeId: proto.String("session-2")}.Build()
	page, err := x.Transcript(t.Context(), resource.SessionTranscriptRequest_builder{Ref: ref}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetEvents()) == 0 {
		t.Fatal("the fixture conversation has no rows to compare")
	}
	versions, err := x.versions(ref, 0)
	if err != nil {
		t.Fatal(err)
	}
	current := map[uint64]*resource.SessionEvent{}
	var order []uint64
	for _, v := range versions {
		if _, seen := current[v.Event.Seq]; !seen {
			order = append(order, v.Event.Seq)
		}
		current[v.Event.Seq] = eventwire.ToResource(v.Event)
	}
	if len(order) != len(page.GetEvents()) {
		t.Fatalf("stream had %d rows, page had %d", len(order), len(page.GetEvents()))
	}
	for i, seq := range order {
		if got, want := current[seq], page.GetEvents()[i]; !proto.Equal(got, want) {
			t.Fatalf("row %d differs\nstream: %v\npage:   %v", seq, got, want)
		}
	}
	// A cursor past the end has nothing to replay.
	if rest, err := x.versions(ref, versions[len(versions)-1].Updated); err != nil || len(rest) != 0 {
		t.Fatal("a spent cursor replayed rows", len(rest), err)
	}
}
