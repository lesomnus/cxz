package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/agentview"
)

// A quota that is polled and found unchanged is recorded as an observation
// rather than as the value again (see cxz#344). It still has to read as a
// current value: a window whose observation stopped moving is rendered as
// approximate, and a session whose limits sit still for an hour would otherwise
// show nothing but approximations.
func TestUnchangedQuotaStaysCurrent(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * time.Minute)
	events := []*api.Event{
		{Kind: "usage", Text: "account/rateLimits/updated", TimeMs: old.UnixMilli(), Payload: []byte(`{"rateLimits":{"primary":{"usedPercent":40,"windowDurationMins":300}}}`)},
	}
	windows, state := quotaSnapshot("codex", "run", events, parseQuotaEvent)
	if state != "available" || len(windows) != 1 {
		t.Fatal("quota not read", state, windows)
	}
	if got := windows[0].Observed; !got.Equal(old.Truncate(time.Millisecond)) {
		t.Fatal("observed at the wrong time", got)
	}

	events = append(events, &api.Event{Kind: "usage_status", Text: agentview.QuotaObserved, TimeMs: now.UnixMilli()})
	windows, state = quotaSnapshot("codex", "run", events, parseQuotaEvent)
	if state != "available" || len(windows) != 1 || *windows[0].Remaining != 60 {
		t.Fatal("the value did not survive its own observation", state, windows)
	}
	if got := windows[0].Observed; !got.Equal(now.Truncate(time.Millisecond)) {
		t.Fatal("an observation did not advance the window", got)
	}

	m := conversationModel()
	m.quotaWindows, m.quotaState = windows, state
	if status := ansi.Strip(m.quotaStatus(now, 40)); strings.Contains(status, "~") {
		t.Fatal("a freshly observed limit was rendered as approximate:", status)
	}
}

// An observation with no value behind it is not a value.
func TestObservationAloneIsNotAQuota(t *testing.T) {
	windows, state := quotaSnapshot("codex", "run", []*api.Event{
		{Kind: "usage_status", Text: agentview.QuotaObserved, TimeMs: time.Now().UnixMilli()},
	}, parseQuotaEvent)
	if len(windows) != 0 || state != "waiting" {
		t.Fatal(state, windows)
	}
}
