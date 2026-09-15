package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/muesli/termenv"
	"google.golang.org/protobuf/proto"
)

func TestFullPermissionSurvivesProjectNavigation(t *testing.T) {
	m := conversationModel()
	s := m.current()
	s.ProjectId = "p1"
	m.project = &api.Project{Id: "p1"}
	m.fullPermission = map[string]string{s.Id: s.RunId}
	m.backToProject()
	if m.fullPermission[s.Id] != s.RunId {
		t.Fatal("navigation reset permission")
	}
	other := &api.Session{Id: "other", ProjectId: "p2", RunId: "other-run", State: "idle"}
	m.project = &api.Project{Id: "p2"}
	m.Update(listing{sessions: []*api.Session{s, other}})
	m.project = &api.Project{Id: "p1"}
	m.projectView = false
	m.Update(listing{sessions: []*api.Session{s, other}})
	if m.fullPermission[s.Id] != s.RunId {
		t.Fatal("return lost permission")
	}
	next := proto.Clone(s).(*api.Session)
	next.RunId = "replacement"
	m.Update(listing{sessions: []*api.Session{next}})
	if m.fullPermission[s.Id] != "" {
		t.Fatal("permission survived run change")
	}
}

func TestPendingInputImmediateAndReconciled(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	m := conversationModel()
	c := &recordingClient{}
	m.client = c
	cmd := m.action("send", "hello immediately")
	if cmd == nil || len(c.inputs) != 0 || !strings.Contains(ansi.Strip(m.view.View()), "> hello immediately") {
		t.Fatal("no local echo before RPC")
	}
	pending := m.pendingInputs["s"][0]
	if !m.isPendingInput(pending) {
		t.Fatal("not marked pending")
	}
	before := m.view.View()
	confirmed := proto.Clone(pending).(*api.Event)
	confirmed.Seq = 1
	m.Update(received{id: "s", event: confirmed})
	after := m.view.View()
	if len(m.pendingInputs["s"]) != 0 || strings.Count(ansi.Strip(after), "> hello immediately") != 1 {
		t.Fatal("duplicate echo")
	}
	if ansi.Strip(before) != ansi.Strip(after) || before == after {
		t.Fatal("confirmation should change only styling")
	}
}

func TestPendingInputFailureAndIdenticalMessages(t *testing.T) {
	m := conversationModel()
	s := m.current()
	m.queueInput(s.Id, s.RunId, "one", "same")
	m.queueInput(s.Id, s.RunId, "two", "same")
	if len(m.transcriptEvents(s.Id)) != 2 {
		t.Fatal("identical texts merged")
	}
	first := proto.Clone(m.pendingInputs[s.Id][0]).(*api.Event)
	first.Seq = 1
	m.Update(received{id: s.Id, event: first})
	if len(m.pendingInputs[s.Id]) != 1 || m.pendingInputs[s.Id][0].RequestId != "two" {
		t.Fatal("wrong input confirmed")
	}
	m.Update(result{inputSession: s.Id, inputRequest: "two", err: errors.New("send failed")})
	if len(m.pendingInputs[s.Id]) != 0 || len(m.events[s.Id]) != 1 {
		t.Fatal("failed input persisted")
	}
}

func TestPinPromptImmediatelyBeforeViewport(t *testing.T) {
	m := conversationModel()
	for i, text := range []string{"prompt one", "prompt two", "prompt three"} {
		m.events["s"] = append(m.events["s"], &api.Event{Seq: uint64(i*2 + 1), Kind: "input", Text: text}, &api.Event{Seq: uint64(i*2 + 2), Kind: "assistant", Text: strings.Repeat("answer\n", 20)})
	}
	m.render()
	m.view.Height = 12
	for i, want := range []string{"prompt one", "prompt two"} {
		m.view.SetYOffset(m.promptSpans[i+1].start - 3)
		view := ansi.Strip(m.conversationView())
		if !strings.HasPrefix(view, "> "+want) || !strings.Contains(view, "> "+[]string{"prompt two", "prompt three"}[i]) {
			t.Fatal("wrong sticky predecessor", view)
		}
	}
	m.view.GotoTop()
	if strings.Count(ansi.Strip(m.conversationView()), "> prompt one") != 1 {
		t.Fatal("visible prompt duplicated")
	}
}
