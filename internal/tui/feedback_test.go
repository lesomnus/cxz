package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestAccountFormArrowNavigation(t *testing.T) {
	m := conversationModel()
	m.openAccounts(false)
	m.accountKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	for i := 1; i <= 4; i++ {
		m.accountKey(tea.KeyMsg{Type: tea.KeyDown})
		if m.accountField != i%4 {
			t.Fatal(m.accountField)
		}
	}
	for _, expected := range []int{3, 2, 1, 0} {
		m.accountKey(tea.KeyMsg{Type: tea.KeyUp})
		if m.accountField != expected {
			t.Fatal(m.accountField)
		}
	}
}

func TestAutomaticApprovalDoesNotFlashPending(t *testing.T) {
	m := conversationModel()
	s := m.current()
	m.fullPermission = map[string]string{s.Id: s.RunId}
	p := &api.Event{Kind: "approval", Text: "Bash", RequestId: "request", RunId: s.RunId, Seq: 1}
	s.Pending = []*api.Event{p}
	m.events[s.Id] = []*api.Event{p}
	m.render()
	if m.selectedApproval() != nil || m.approvalBox() != "" || strings.Contains(ansi.Strip(m.view.View()), "[ ] Bash") {
		t.Fatal("automatic pending flashed")
	}
	if !m.hiddenAutoApproval(s, p) {
		t.Fatal("auto request not hidden")
	}
	question := &api.Event{Kind: "approval", Text: "AskUserQuestion", RequestId: "q", RunId: s.RunId}
	s.Pending = append(s.Pending, question)
	if m.selectedApproval() != question {
		t.Fatal("question hidden")
	}
	m.events[s.Id] = append(m.events[s.Id], &api.Event{Kind: "approval_resolved", Text: "allowed", RequestId: p.RequestId, RunId: s.RunId, Seq: 2})
	m.render()
	if !strings.Contains(ansi.Strip(m.view.View()), "[✓] Bash") {
		t.Fatal(m.view.View())
	}
	delete(m.fullPermission, s.Id)
	m.events[s.Id] = []*api.Event{p}
	m.render()
	if m.selectedApproval() != p || !strings.Contains(ansi.Strip(m.view.View()), "[ ] Bash") {
		t.Fatal("manual fallback hidden")
	}
}

func TestQuotaPollingAgesToTimeout(t *testing.T) {
	now := time.Now()
	events := []*api.Event{{Kind: "usage_status", Text: "polling", RunId: "r", TimeMs: now.Add(-2 * time.Minute).UnixMilli()}}
	if quotaPendingState("polling", "r", events, now) != "timeout" {
		t.Fatal("polling forever")
	}
	if quotaPendingState("available", "r", events, now) != "available" || quotaPendingState("polling", "other", events, now) != "polling" {
		t.Fatal("wrong state/run affected")
	}
}

func TestOpenModelPickerReceivesRefreshedCatalog(t *testing.T) {
	m := conversationModel()
	m.modelPicker = &modelPicker{id: "s", run: "run", kind: "/model", catalog: &modelCatalog{}}
	m.Update(received{id: "s", event: &api.Event{Seq: 100, RunId: "run", Kind: "models", Payload: []byte(`{"models":[{"id":"fresh-model"}]}`)}})
	if options := m.modelPicker.options(); len(options) != 1 || options[0] != "fresh-model" {
		t.Fatal(options)
	}
}
