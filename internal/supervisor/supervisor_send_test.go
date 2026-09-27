package supervisor

import (
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

// The lifecycle test retries a send the supervisor refuses, because a projection
// can report an idle that predates the previous turn. That is only safe while the
// refusal happens before anything is written: nothing reaches the provider and
// nothing lands in the journal, so a retry cannot duplicate a prompt.
func TestRefusedSendWritesNothing(t *testing.T) {
	s, wire := displaySupervisor(t, "claude")
	s.snap = Replay([]core.Event{{Kind: "state", Text: "working", RunID: "run"}})
	before := len(s.log.All())
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "busy", Text: "hello"}); err == nil {
		t.Fatal("send accepted while the session was working")
	}
	if wire.Len() != 0 {
		t.Fatal("refused send reached the provider", wire.String())
	}
	if len(s.log.All()) != before {
		t.Fatal("refused send recorded an intent")
	}
	s.event("state", "idle", "", nil, nil)
	if _, err := s.execute("send", core.Command{RunID: "run", ClientID: "ready", Text: "hello"}); err != nil {
		t.Fatal("retry refused once idle", err)
	}
	if wire.Len() == 0 {
		t.Fatal("accepted send never reached the provider")
	}
}
