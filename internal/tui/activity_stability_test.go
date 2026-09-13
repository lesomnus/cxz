package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestActivityRowStableAcrossAutoApprovalStates(t *testing.T) {
	m := conversationModel()
	s := m.current()
	m.fullPermission = map[string]string{s.Id: s.RunId}
	started := time.Now().Add(-21 * time.Second).UnixMilli()
	m.events[s.Id] = []*api.Event{{Seq: 1, RunId: s.RunId, Kind: "input", Text: "Edit files", TimeMs: started}}
	for i := 0; i < 30; i++ {
		m.events[s.Id] = append(m.events[s.Id], &api.Event{Seq: uint64(i + 2), RunId: s.RunId, Kind: "assistant", Text: fmt.Sprintf("Response %d", i)})
	}
	var height, total, offset, row int
	for i, state := range []string{"working", "waiting_input", "working", "waiting_input", "working"} {
		s.State = state
		s.Pending = nil
		if state == "waiting_input" {
			s.Pending = []*api.Event{{Text: "Edit", RunId: s.RunId, RequestId: "permission"}}
		}
		m.resize()
		m.render()
		m.view.GotoBottom()
		rows := strings.Split(ansi.Strip(m.conversationView()), "\n")
		index := -1
		for j, line := range rows {
			if strings.Contains(line, "working ") {
				index = j
			}
		}
		if index < 0 || m.approvalHeight() != 0 || m.workingSince != started {
			t.Fatalf("indicator disappeared/reset in %s: %v", state, rows)
		}
		if i == 0 {
			height, total, offset, row = m.view.Height, len(m.historyTimes), m.view.YOffset, index
		} else if height != m.view.Height || total != len(m.historyTimes) || offset != m.view.YOffset || row != index {
			t.Fatalf("layout moved in %s", state)
		}
	}
	s.State = "idle"
	m.render()
	if strings.Contains(ansi.Strip(m.conversationView()), "Esc interrupt") {
		t.Fatal("idle spinner remains")
	}
}

func TestWaitingActivityUsesSameRowAndElapsedTime(t *testing.T) {
	m := conversationModel()
	s := m.current()
	s.State = "waiting_input"
	now := time.Unix(100, 0)
	m.workingSince = now.Add(-21 * time.Second).UnixMilli()
	for _, tc := range []struct{ tool, label string }{{"Write", "waiting for approval"}, {"AskUserQuestion", "waiting for answer"}} {
		s.Pending = []*api.Event{{Text: tc.tool, RunId: s.RunId, RequestId: "q"}}
		if !m.activeWork() || !strings.Contains(m.workingLabel(now), tc.label+" 00:00:21") {
			t.Fatal(m.workingLabel(now))
		}
	}
	s.State = "stopped"
	if m.activeWork() {
		t.Fatal("stopped session marked active")
	}
}
