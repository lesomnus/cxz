package supervisor

import (
	"testing"
	"time"
)

// A client that reports itself busy refuses this attempt and nothing more. It
// used to restart the idle period, and a TUI calls itself busy for ten seconds
// after any keypress -- so reading a conversation kept an installation from
// ever updating. See cxz#224.
func TestABusyClientRefusesWithoutRestartingTheClock(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	quiet := time.Now()
	s.lastActivity = quiet.Add(-2 * UpdateIdlePeriod)
	s.updateClients = map[string]updateClient{"tui": {seen: quiet, busy: true}}
	if got := s.updateReason(quiet, true); got != "attached client busy" {
		t.Fatal("a busy client did not refuse:", got)
	}
	if !s.lastActivity.Equal(quiet.Add(-2 * UpdateIdlePeriod)) {
		t.Fatal("the busy client moved the idle clock")
	}
	// The moment it stops saying so, the session is already quiet: no further
	// five minutes to serve.
	s.updateClients["tui"] = updateClient{seen: quiet, busy: false}
	if got := s.updateReason(quiet, true); got != "" {
		t.Fatal("a session that had been quiet all along was made to wait again:", got)
	}
}

// A client that has not been heard from for a while is forgotten, and that is
// an interaction ending rather than one continuing -- so the clock does start.
func TestAVanishedClientRestartsTheClock(t *testing.T) {
	s, _ := displaySupervisor(t, "claude")
	now := time.Now()
	s.lastActivity = now.Add(-2 * UpdateIdlePeriod)
	s.updateClients = map[string]updateClient{"gone": {seen: now.Add(-time.Minute), busy: false}}
	if got := s.updateReason(now, true); got == "" {
		t.Fatal("forgetting a client did not restart the idle period")
	}
	if _, still := s.updateClients["gone"]; still {
		t.Fatal("a client not seen for a minute was kept")
	}
}
