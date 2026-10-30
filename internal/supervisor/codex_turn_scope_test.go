package supervisor

import (
	"fmt"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

func TestCodexForeignThreadDoesNotFinishTheActiveTurn(t *testing.T) {
	s, _ := displaySupervisor(t, "codex")
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "input", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"method":"turn/started","params":{"threadId":"thread","turn":{"id":"main"}}}`))
	s.consume([]byte(`{"method":"item/completed","params":{"threadId":"thread","turnId":"main","item":{"id":"progress","type":"agentMessage","phase":"commentary","text":"still working"}}}`))
	s.consume([]byte(`{"id":"approval","method":"item/commandExecution/requestApproval","params":{"threadId":"thread","turnId":"main","command":"main command"}}`))
	last := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	s.codex.progress.last = last
	before := s.snap.LastSeq
	for _, raw := range []string{
		`{"method":"turn/started","params":{"threadId":"child","turn":{"id":"child-turn"}}}`,
		`{"method":"item/started","params":{"threadId":"child","turnId":"child-turn","item":{"id":"child-tool","type":"commandExecution","command":"child command"}}}`,
		`{"method":"item/completed","params":{"threadId":"child","turnId":"child-turn","item":{"id":"child-answer","type":"agentMessage","phase":"final_answer","text":"child done"}}}`,
		`{"method":"turn/completed","params":{"threadId":"child","turn":{"id":"child-turn","status":"completed"}}}`,
	} {
		s.consume([]byte(raw))
	}
	if s.codex.turn != "main" || s.responseContext.TurnID != "main" || s.snap.State != "waiting_input" || len(s.pending) != 1 {
		t.Fatalf("foreign turn changed the active turn: turn=%q response=%+v state=%q pending=%v", s.codex.turn, s.responseContext, s.snap.State, s.pending)
	}
	if !s.codex.progress.last.Equal(last) {
		t.Fatal("foreign work reset the active turn's progress")
	}
	for _, e := range s.log.All() {
		if e.Seq > before && e.Kind != "raw" {
			t.Fatalf("foreign notification escaped the raw journal: %+v", e)
		}
	}
	s.consume([]byte(`{"method":"item/completed","params":{"threadId":"thread","turnId":"main","item":{"id":"answer","type":"agentMessage","phase":"final_answer","text":"main done"}}}`))
	s.consume([]byte(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"main","status":"completed","durationMs":1234}}}`))
	if s.snap.State != "idle" || s.codex.turn != "" || len(s.pending) != 0 {
		t.Fatal("matching turn did not finish", s.snap)
	}
	if got := completion(t, s); got.ResponseSeq != assistantEvents(s)[1].Seq || *got.DurationMS != 1234 {
		t.Fatal("completion did not belong to the main response", got)
	}
}

func TestCodexStaleAndDuplicateTurnCompletionDoNotFinishCurrentWork(t *testing.T) {
	for _, thread := range []string{"", `"threadId":"thread",`} {
		t.Run(fmt.Sprintf("thread=%s", thread), func(t *testing.T) {
			s, _ := displaySupervisor(t, "codex")
			s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"old"}}}`))
			s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"current"}}}`))
			s.consume([]byte(`{"method":"item/completed","params":{"turnId":"current","item":{"id":"answer","type":"agentMessage","phase":"final_answer","text":"done"}}}`))
			last, before := s.codex.progress.last, s.snap.LastSeq
			s.consume([]byte(`{"method":"turn/completed","params":{` + thread + `"turn":{"id":"old","status":"completed"}}}`))
			if s.codex.turn != "current" || s.snap.State != "working" || !s.codex.progress.last.Equal(last) {
				t.Fatal("stale completion finished current work", s.snap)
			}
			for _, e := range s.log.All() {
				if e.Seq > before && e.Kind != "raw" {
					t.Fatalf("stale completion produced a notification candidate: %+v", e)
				}
			}
			finished := []byte(`{"method":"turn/completed","params":{` + thread + `"turn":{"id":"current","status":"completed"}}}`)
			s.consume(finished)
			last, before = s.codex.progress.last, s.snap.LastSeq
			s.consume(finished)
			if s.snap.State != "idle" || !s.codex.progress.last.Equal(last) {
				t.Fatal("duplicate completion changed state", s.snap)
			}
			for _, e := range s.log.All() {
				if e.Seq > before && e.Kind != "raw" {
					t.Fatalf("duplicate completion produced a second notification candidate: %+v", e)
				}
			}
			_ = completion(t, s)
		})
	}
}
