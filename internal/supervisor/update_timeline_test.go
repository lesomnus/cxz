package supervisor

import (
	"fmt"
	"testing"
	"time"
)

// An hour of a session that nobody is typing into, with one browser tab
// attached. This is the shape measured on the installation: eight Events
// streams in forty-seven minutes, a client ID minted per stream, against a five
// minute idle period. Each stream used to leave an attach reset and an expiry
// reset behind it, so the session reported "waiting for 5 minutes of continuous
// idle" for fifty minutes and refused the rollout four times per window while
// every other project was ready.
//
// Written as a timeline because that is the only way the bug is visible: every
// single answer along it is correct on its own. See cxz#249.
func TestAnHourOfAReconnectingViewerStillAllowsAnUpdate(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	start := time.Now()
	s.snap.State = "idle"
	// The session last did something of its own at the start of the hour.
	s.lastActivity = start
	s.updateClients = map[string]updateClient{}

	eligible := time.Time{}
	for minute := 0; minute < 60; minute++ {
		now := start.Add(time.Duration(minute) * time.Minute)
		// A stream lives about six minutes and comes back with a new identity,
		// reporting itself busy once, the way the Events watch does.
		if minute%6 == 0 {
			s.updateClients[fmt.Sprintf("tab-%d", minute)] = updateClient{seen: now, busy: true}
		}
		reason := s.updateReason(now, "")
		if reason == "" && eligible.IsZero() {
			eligible = now
		}
		if reason != "" && !eligible.IsZero() && reason != "attached client busy" {
			t.Fatalf("minute %d: a session that had become quiet was made to wait again: %q", minute, reason)
		}
	}
	if eligible.IsZero() {
		t.Fatal("an idle session with a tab attached never became eligible in an hour")
	}
	if waited := eligible.Sub(start); waited > 10*time.Minute {
		t.Fatal("the session took", waited, "to settle; the idle period is", UpdateIdlePeriod)
	}
}
