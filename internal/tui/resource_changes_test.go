package tui

import (
	"testing"
	"time"
)

func TestResourceRefreshCadenceAndSingleFlight(t *testing.T) {
	m := conversationModel()
	m.lastResourceRefresh = time.Now()
	for i := 0; i < 20; i++ {
		if m.periodicRefresh() != nil {
			t.Fatal("visual tick polled resources")
		}
	}
	m.lastResourceRefresh = time.Now().Add(-31 * time.Second)
	if m.periodicRefresh() == nil {
		t.Fatal("missing safety refresh")
	}
	if m.refresh() != nil || !m.resourceRefreshAgain {
		t.Fatal("overlapping refresh not coalesced")
	}
}

func TestResourceChangeBurstCoalesces(t *testing.T) {
	m := conversationModel()
	_, cmd := m.update(resourcesChanged{})
	if cmd == nil {
		t.Fatal("missing debounce")
	}
	for i := 0; i < 50; i++ {
		_, cmd = m.update(resourcesChanged{})
		if cmd != nil {
			t.Fatal("burst scheduled duplicate refresh")
		}
	}
	_, cmd = m.update(resourcesRefreshDue{})
	if cmd == nil || !m.resourceRefreshRunning || m.resourceRefreshPending {
		t.Fatal("refresh state")
	}
}

func TestOldResourceSubscriptionCannotResetNewOne(t *testing.T) {
	m := conversationModel()
	m.resourceWatchGeneration = 2
	m.resourcesWatching = true
	m.update(resourcesWatchEnded{generation: 1})
	if !m.resourcesWatching {
		t.Fatal("stale disconnect reset new watch")
	}
	_, cmd := m.update(resourcesChanged{generation: 1})
	if cmd != nil || m.resourceRefreshPending {
		t.Fatal("stale watch invalidated list")
	}
}
