package supervisor

import (
	"errors"
	"fmt"

	"github.com/lesomnus/cxz/internal/core"
)

// errQueueInput is how a protocol says it cannot take this message yet without
// also saying the message is wrong.
var errQueueInput = errors.New("agent cannot take a message yet")

// One slot, held by cxz rather than by the agent process. A slot is enough to
// keep typing while a turn runs, and small enough that what is waiting is one
// nameable thing: it can be shown, cancelled, and reported when it is dropped.
// Written into the CLI instead, it would be unreachable the moment it was sent.
//
// The slot empties when the agent can take a message, which each agent defines
// for itself: Claude at the end of its turn, Codex at the next step of the one
// running. Everything above this is the same for both.

func (s *Supervisor) enqueue(c core.Command) (core.Receipt, error) {
	receipt := core.Receipt{ClientID: c.ClientID}
	// Only an agent that is running can come back for the message. Anywhere
	// else, holding it would be a promise nothing is going to keep.
	if s.stopping || (s.snap.State != "working" && s.snap.State != "waiting_input" && s.snap.State != "starting") {
		return receipt, errors.New("session is " + s.snap.State + "; no agent will take a message")
	}
	if s.snap.Queued != "" {
		return receipt, errors.New("a message is already waiting for the agent; cancel it or wait")
	}
	r := record{Op: "send", Command: c, Status: "queued"}
	s.event("queued", c.Text, c.ClientID, nil, nil)
	s.receipts[c.ClientID] = r
	receipt.Status = "queued"
	return receipt, nil
}

// unqueue takes back the waiting message. The text is not repeated into the
// journal here -- the queued event already holds it -- so this records only
// that nothing is waiting any more.
func (s *Supervisor) unqueue(c core.Command) (core.Receipt, error) {
	receipt := core.Receipt{ClientID: c.ClientID}
	if s.snap.Queued == "" {
		return receipt, errors.New("no message is waiting")
	}
	r := record{Op: "unqueue", Command: c, Status: "accepted"}
	s.event("queued", "", "", r, nil)
	s.receipts[c.ClientID] = r
	receipt.Status = "accepted"
	return receipt, nil
}

// deliverQueued runs where approvePending runs: under the lock, after the
// agent's batch is durable, so the state it reads is the state the journal has.
func (s *Supervisor) deliverQueued() {
	if s.snap.Queued == "" || s.stopping || s.snap.State != "idle" {
		return
	}
	// A steer that has not been answered may still take this message. Sending
	// it again here is how one message becomes two.
	if s.codex != nil && len(s.codex.steering) > 0 {
		return
	}
	c := core.Command{RunID: s.snap.RunID, ClientID: s.snap.QueuedID, Text: s.snap.Queued}
	// The queued receipt answered the client that sent it. Delivery is the same
	// command arriving at the agent, so the receipt it replaces is this one; no
	// caller can observe the gap, which is inside this lock.
	delete(s.receipts, c.ClientID)
	if _, err := s.executeLocked("send", c); err != nil {
		s.dropQueued("delivery failed: " + err.Error())
	}
}

// dropQueued clears the slot and says so. A message the user wrote is never
// discarded quietly: the diagnostic is what the frontend turns into a notice
// and what a journal reader finds later.
func (s *Supervisor) dropQueued(reason string) {
	if s.snap.Queued == "" {
		return
	}
	text := s.snap.Queued
	s.event("queued", "", "", nil, nil)
	s.event("diagnostic", fmt.Sprintf("the message waiting to be sent was dropped because %s: %q", reason, text), "", nil, nil)
}
