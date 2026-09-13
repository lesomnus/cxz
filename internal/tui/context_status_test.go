package tui

import (
	"github.com/lesomnus/cxz/api"
	"testing"
)

func TestContextDots(t *testing.T) {
	for _, tc := range []struct {
		p    int
		want string
	}{{0, "⠀0%"}, {10, "⠀10%"}, {11, "⡀11%"}, {22, "⣀22%"}, {35, "⣄35%"}, {44, "⣤44%"}, {55, "⣦55%"}, {66, "⣶66%"}, {77, "⣷77%"}, {88, "⣿88%"}, {100, "⣿99%"}} {
		if got := contextBadge(tc.p, true); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	if got := contextBadge(0, false); got != "⠀—" {
		t.Fatal(got)
	}
}

func TestContextStatusRunAndCompaction(t *testing.T) {
	m := conversationModel()
	s := m.current()
	s.Agent = "codex"
	m.events[s.Id] = []*api.Event{{RunId: s.RunId, Kind: "usage", Text: "thread/tokenUsage/updated", Payload: []byte(`{"tokenUsage":{"last":{"totalTokens":47000},"modelContextWindow":112000}}`)}}
	if got := m.contextStatus(); got != "⣄35%" {
		t.Fatal(got)
	}
	m.events[s.Id] = append(m.events[s.Id], &api.Event{RunId: s.RunId, Kind: "compact"})
	if got := m.contextStatus(); got != "⠀—" {
		t.Fatal(got)
	}
	s.RunId = "new-run"
	if got := m.contextStatus(); got != "⠀—" {
		t.Fatal(got)
	}
}
