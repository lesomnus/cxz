package transcripthistory

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/api"
)

// turns writes n closed turns followed by one open input, and returns the seq
// the open turn starts at.
func turns(write func(...*api.Event), n int) (seq uint64, open uint64) {
	for i := 0; i < n; i++ {
		input := event(seq+1, "input", "", "")
		assistant := event(seq+2, "assistant", "", `{"item":{"phase":"final_answer"}}`)
		assistant.Response = &api.ResponseMetadata{Model: "model", Phase: "final_answer"}
		end := event(seq+3, "turn_end", "", `{"duration_ms":10}`)
		end.Text = "completed"
		write(input, assistant, end)
		seq += 3
	}
	open = seq + 1
	write(event(open, "input", "", ""))
	return seq + 1, open
}

// A restore has to carry what a reduction can still need, and what it can still
// need is inside the turn that has not closed. Reading every row of the
// conversation to find that was the cost during an active conversation. See
// docs/plans/incremental-projection.md.
func TestColdRestoreReadsTheOpenTurnNotTheConversation(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	seq, open := turns(write, 6)
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	// A cold look at a changed conversation: no cache, and not a rebuild.
	write(event(seq+1, "assistant", "", ""))
	at, rows := restored.Load(), restoredRows.Load()
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	if restored.Load() != at+1 {
		t.Fatal("expected one restore", restored.Load()-at)
	}
	if read := restoredRows.Load() - rows; read > 2 {
		t.Fatal("a restore read past the open turn:", read, "rows for a turn that starts at", open)
	}

	// What it produced is what a full replay produces.
	again, writeAgain := fixture(t)
	turns(writeAgain, 6)
	writeAgain(event(seq+1, "assistant", "", ""))
	if err := Sync(ctx, again, "s", decode); err != nil {
		t.Fatal(err)
	}
	want, err := Tail(ctx, again, "s", 0, 4096, decode)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Tail(ctx, db, "s", 0, 4096, decode)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatal("bounded restore and full replay disagree", len(got), len(want))
	}
	for i := range got {
		if got[i].GetEvent().GetSeq() != want[i].GetEvent().GetSeq() {
			t.Fatal("row differs at", i, got[i], want[i])
		}
	}
}

// A background task answers after its turn closed, so its call is behind what
// any bounded restore carries. One indexed look finds it.
func TestAResultFromAClosedTurnFindsItsCall(t *testing.T) {
	db, write := fixture(t)
	ctx := context.Background()
	call := event(2, "tool_call", "bg", `{"item":{"type":"commandExecution","command":"sleep 1","status":"inProgress"}}`)
	call.Text = "Bash"
	assistant := event(3, "assistant", "", `{"item":{"phase":"final_answer"}}`)
	assistant.Response = &api.ResponseMetadata{Model: "model", Phase: "final_answer"}
	end := event(4, "turn_end", "", `{"duration_ms":10}`)
	end.Text = "completed"
	write(event(1, "input", "", ""), call, assistant, end)
	write(event(5, "input", "", "")) // a new turn, which is where the floor now is
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}

	at := resolved.Load()
	write(event(6, "tool_result", "bg", `{"content":"done"}`))
	if err := Sync(ctx, db, "s", decode); err != nil {
		t.Fatal(err)
	}
	if resolved.Load() != at+1 {
		t.Fatal("the call was not looked up", resolved.Load()-at)
	}
	rows, err := Tail(ctx, db, "s", 0, 4096, decode)
	if err != nil {
		t.Fatal(err)
	}
	var anchor *api.Event
	for _, r := range rows {
		if r.GetEvent().GetSeq() == 2 {
			anchor = r.GetEvent()
		}
	}
	if anchor == nil || anchor.GetToolSummary() == nil {
		t.Fatal("no row for the tool call", rows)
	}
	if anchor.GetToolSummary().GetName() != "Bash" {
		t.Fatal("the result replaced the call instead of completing it", anchor.GetToolSummary())
	}
	if state := anchor.GetToolSummary().GetState(); state != "completed" {
		t.Fatal("the result did not complete the call", state)
	}
}
