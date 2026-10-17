package websandbox

import (
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/internal/transcripthistory"
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

// A tool's records become one row whose state advances, which is the shape
// every client renders. Pinned here because the browser suites cannot reach it
// deterministically: catching the working state in a browser is a race against
// the sandbox's own pacing.
func TestToolRecordsBecomeOneAdvancingRow(t *testing.T) {
	s := New(1, time.Millisecond)
	defer s.Close()
	x := s.Sessions()
	ref := resource.SessionRef_builder{RuntimeId: proto.String("session-9")}.Build()
	versions, err := x.versions(ref, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Grouped by row, because the scenario seeds two tools: the Bash call whose
	// state advances, and a Skill whose name is not what it is.
	byRow := map[uint64][]transcripthistory.Version{}
	var order []uint64
	for _, v := range versions {
		if v.Event.GetToolSummary() == nil {
			continue
		}
		if _, seen := byRow[v.Seq]; !seen {
			order = append(order, v.Seq)
		}
		byRow[v.Seq] = append(byRow[v.Seq], v)
	}
	if len(order) != 2 {
		t.Fatalf("the scenario published %d tool rows, want the command and the skill", len(order))
	}
	tools := byRow[order[0]]
	// Two versions, not three: the call opens the row and the result closes it,
	// while the output in between only moves the state to working and is then
	// deduplicated. That bound is what keeps a chatty tool from republishing its
	// row once per chunk.
	if len(tools) != 2 {
		t.Fatalf("the seeded tool published %d row versions, want 2", len(tools))
	}
	if got := tools[0].Event.GetToolSummary().GetState(); got == "completed" {
		t.Fatal("the call already read as completed")
	}
	if tools[1].Updated <= tools[0].Updated {
		t.Fatal("the completion did not arrive after the row it completed")
	}
	last := tools[1].Event.GetToolSummary()
	if last.GetState() != "completed" {
		t.Fatal("the result did not complete the row:", last.GetState())
	}
	if last.GetCommand() == "" {
		t.Fatal("the row lost the command it ran")
	}
	if last.GetKind() != "command" {
		t.Fatal("the row did not say it ran a command:", last.GetKind())
	}
	// "Skill" names every skill. Which one, and with what arguments, is only in
	// the description -- without it this row reads as the word "Skill".
	skill := byRow[order[1]][0].Event.GetToolSummary()
	if skill.GetKind() != "tool" || skill.GetDescription() != "Skill dataviz palette" {
		t.Fatal("the skill row lost what it is:", skill.GetKind(), skill.GetDescription())
	}
	// Output records are not rows of their own, which is why a reader that wants
	// the bytes reads the records rather than the projection.
	for _, v := range versions {
		if k := v.Event.GetKind(); k == "tool_output" || k == "tool_result" {
			t.Fatal("an output record became a row", k)
		}
	}
}
