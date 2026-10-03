package supervisor

import (
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func kinds(events []core.Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Kind)
	}
	return out
}

func lastOf(events []core.Event, kind string) (core.Event, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == kind {
			return events[i], true
		}
	}
	return core.Event{}, false
}

// A message sent into a running turn waits in cxz rather than being refused or
// written where it cannot be taken back. Nothing reaches the provider until the
// turn ends, and then exactly one message does.
func TestClaudeSendWaitsForTheTurn(t *testing.T) {
	s, wire := displaySupervisor(t, "claude")
	s.snap = Replay([]core.Event{{Kind: "state", Text: "working", RunID: "run"}})
	v, err := s.execute("send", core.Command{RunID: "run", ClientID: "waiting", Text: "also fix the test"})
	if err != nil || v.Status != "queued" {
		t.Fatal("send into a turn was not queued:", v, err)
	}
	if wire.Len() != 0 {
		t.Fatal("a waiting message reached the provider", wire.String())
	}
	if e, ok := lastOf(s.log.All(), "queued"); !ok || e.Text != "also fix the test" || e.RequestID != "waiting" {
		t.Fatal("the journal does not say what is waiting:", kinds(s.log.All()))
	}
	if s.snap.Queued != "also fix the test" {
		t.Fatal("the projection does not carry the waiting message:", s.snap.Queued)
	}
	// One slot: a second message is refused, and the sender keeps its text.
	if _, err = s.execute("send", core.Command{RunID: "run", ClientID: "second", Text: "and this"}); err == nil {
		t.Fatal("a second message was accepted into a full slot")
	}
	// The same send retried is the same message, not another one.
	if v, err = s.execute("send", core.Command{RunID: "run", ClientID: "waiting", Text: "also fix the test"}); err != nil || v.Status != "queued" {
		t.Fatal("retrying the queued send:", v, err)
	}
	if wire.Len() != 0 {
		t.Fatal("a retry reached the provider", wire.String())
	}
	s.consume([]byte(`{"type":"result","is_error":false,"result":"done"}`))
	if !strings.Contains(wire.String(), "also fix the test") {
		t.Fatal("the waiting message was not delivered when the turn ended", wire.String())
	}
	if strings.Count(wire.String(), "also fix the test") != 1 {
		t.Fatal("the waiting message was delivered more than once", wire.String())
	}
	if e, ok := lastOf(s.log.All(), "input"); !ok || e.Text != "also fix the test" || e.RequestID != "waiting" {
		t.Fatal("delivery did not record the message as input:", kinds(s.log.All()))
	}
	if s.snap.Queued != "" {
		t.Fatal("the slot is still full after delivery:", s.snap.Queued)
	}
	// The journal is the whole story: a restart reads the same empty slot.
	if replayed := Replay(s.log.All()); replayed.Queued != "" {
		t.Fatal("replay resurrected a delivered message:", replayed.Queued)
	}
}

// Codex takes input into the turn it is running. The message is only in the
// conversation once Codex says it took it, because the turn it names can end
// first -- and then it waits for the next turn instead of being lost.
func TestCodexSendSteersTheRunningTurn(t *testing.T) {
	s, wire := displaySupervisor(t, "codex")
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn-1"}}}`))
	if s.snap.State != "working" {
		t.Fatal("fixture did not start a turn:", s.snap.State)
	}
	v, err := s.execute("send", core.Command{RunID: "run", ClientID: "steer", Text: "use the other file"})
	if err != nil || v.Status != "accepted" {
		t.Fatal("steering was not accepted:", v, err)
	}
	if !strings.Contains(wire.String(), `"turn/steer"`) || !strings.Contains(wire.String(), `"turnId":"turn-1"`) {
		t.Fatal("the message did not steer the running turn:", wire.String())
	}
	if _, ok := lastOf(s.log.All(), "input"); ok {
		t.Fatal("a steered message entered the conversation before Codex took it")
	}
	if s.snap.Queued != "use the other file" {
		t.Fatal("a steered message is not waiting:", s.snap.Queued)
	}
	s.consume([]byte(`{"id":"steer","result":{"turnId":"turn-1"}}`))
	if e, ok := lastOf(s.log.All(), "input"); !ok || e.Text != "use the other file" {
		t.Fatal("the acknowledged message did not enter the conversation:", kinds(s.log.All()))
	}
	if s.snap.Queued != "" || s.snap.State != "working" {
		t.Fatal("after acknowledgement:", s.snap.Queued, s.snap.State)
	}
}

// A turn that ends while the steer is in flight refuses it. The message is
// still in the slot, so it goes out as the next turn rather than being lost or
// turning a finished turn into a failed one.
func TestCodexRefusedSteeringWaitsForTheNextTurn(t *testing.T) {
	s, wire := displaySupervisor(t, "codex")
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn-1"}}}`))
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "steer", Text: "late"}); err != nil {
		t.Fatal(err)
	}
	s.consume([]byte(`{"method":"turn/completed","params":{"turn":{"id":"turn-1","status":"completed"}}}`))
	// The turn ended first, but the steer has not been answered: sending the
	// message again now is how one message becomes two.
	if strings.Contains(wire.String(), `"turn/start"`) {
		t.Fatal("the message went out again while the steer was unanswered", wire.String())
	}
	s.consume([]byte(`{"id":"steer","error":{"code":-32602,"message":"no active turn"}}`))
	if e, ok := lastOf(s.log.All(), "turn_end"); !ok || e.Text != "completed" {
		t.Fatal("the refused steer rewrote the finished turn:", kinds(s.log.All()))
	}
	// Once refused, the message is free to become a turn of its own.
	if !strings.Contains(wire.String(), `"turn/start"`) || !strings.Contains(wire.String(), "late") {
		t.Fatal("the refused message never became a turn:", wire.String())
	}
	if s.snap.Queued != "" {
		t.Fatal("the slot is still full:", s.snap.Queued)
	}
	if strings.Count(wire.String(), "late") != 2 {
		t.Fatal("the message was written other than once as a steer and once as a turn", wire.String())
	}
}

// Interrupting is changing direction, so what was written for the old one does
// not arrive the moment the turn stops. It is reported, not dropped in silence.
func TestInterruptDropsTheWaitingMessage(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.snap = Replay([]core.Event{{Kind: "state", Text: "working", RunID: "run"}})
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "waiting", Text: "never mind"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.execute("interrupt", core.Command{RunID: "run", ClientID: "stop-it"}); err != nil {
		t.Fatal(err)
	}
	if s.snap.Queued != "" {
		t.Fatal("the interrupt kept the waiting message:", s.snap.Queued)
	}
	e, ok := lastOf(s.log.All(), "diagnostic")
	if !ok || !strings.Contains(e.Text, "never mind") {
		t.Fatal("the dropped message was not reported:", kinds(s.log.All()))
	}
}

// Cancelling takes the message back, and says nothing is waiting without
// rewriting what the journal already recorded.
func TestCancelTakesTheMessageBack(t *testing.T) {
	s, wire := displaySupervisor(t, "claude")
	s.snap = Replay([]core.Event{{Kind: "state", Text: "working", RunID: "run"}})
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "waiting", Text: "wrong file"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.execute("unqueue", core.Command{RunID: "run", ClientID: "take-it-back"}); err != nil {
		t.Fatal(err)
	}
	if s.snap.Queued != "" || Replay(s.log.All()).Queued != "" {
		t.Fatal("the message is still waiting")
	}
	s.consume([]byte(`{"type":"result","is_error":false,"result":"done"}`))
	if strings.Contains(wire.String(), "wrong file") {
		t.Fatal("a cancelled message was delivered anyway", wire.String())
	}
	if _, err := s.execute("unqueue", core.Command{RunID: "run", ClientID: "again"}); err == nil {
		t.Fatal("cancelling nothing reported success")
	}
}

// An idle session still sends immediately: the slot exists for the times it
// cannot, and must not add a round trip to the ordinary case.
func TestIdleSendIsNotQueued(t *testing.T) {
	s, wire := displaySupervisor(t, "claude")
	v, err := s.execute("send", core.Command{RunID: "run", ClientID: "now", Text: "hello"})
	if err != nil || v.Status != "accepted" {
		t.Fatal("idle send:", v, err)
	}
	if !strings.Contains(wire.String(), "hello") {
		t.Fatal("an idle send did not reach the provider", wire.String())
	}
	if s.snap.Queued != "" {
		t.Fatal("an idle send went to the slot")
	}
}

// Holding a message for an agent that is not running would be a promise nothing
// is going to keep, so that send is refused and the sender keeps its text.
func TestStoppedSessionRefusesRatherThanHolds(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	s.snap = Replay([]core.Event{{Kind: "state", Text: "stopped", RunID: "run"}})
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "late", Text: "hello"}); err == nil {
		t.Fatal("a stopped session took the message")
	}
	if s.snap.Queued != "" {
		t.Fatal("a stopped session is holding a message:", s.snap.Queued)
	}
}
