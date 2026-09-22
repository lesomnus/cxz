package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func deleteModel() *model {
	m := projectModel()
	m.sessions = []*api.Session{{Id: "target", Alias: "cedar", RunId: "run", ProjectId: "p"}, {Id: "other", ProjectId: "p"}}
	m.panelFocus, m.panelIndex = true, 1
	return m
}

func TestSessionDeleteThreeSecondWindow(t *testing.T) {
	for _, delay := range []time.Duration{3*time.Second - time.Nanosecond, 3 * time.Second, 4 * time.Second} {
		m := deleteModel()
		now := time.Now()
		if m.confirmSessionDelete(now) != nil {
			t.Fatal("first key deleted session")
		}
		cmd := m.confirmSessionDelete(now.Add(delay))
		if delay < 3*time.Second {
			if cmd == nil {
				t.Fatal("second key within window did not delete")
			}
		} else {
			if cmd != nil || m.deleteConfirm == nil || m.deleteConfirm.until != now.Add(delay+3*time.Second) {
				t.Fatal("expired confirmation was reused")
			}
			cmd = m.confirmSessionDelete(now.Add(delay + time.Second))
			if cmd == nil {
				t.Fatal("fresh pair did not delete")
			}
		}
		m.receiveSessionDeleted(cmd().(sessionDeleted))
		if m.deletingID != "" || m.deleteConfirm != nil || len(m.client.(*projectClient).deleted) != 1 {
			t.Fatal("delete did not finish")
		}
	}
}

func TestSessionDeleteConfirmationVisibleAndExpires(t *testing.T) {
	for _, width := range []int{60, 70, 200} {
		m := deleteModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
		if view := ansi.Strip(m.View()); !strings.Contains(view, "Ctrl+X again · delete (3s)") {
			t.Fatal("confirmation hidden behind focused panel border", width, view)
		}
		m.updateDeleteConfirmation(pulseTick{}, m.deleteConfirm.until)
		if m.deleteConfirm != nil || strings.Contains(ansi.Strip(m.View()), "Ctrl+X again") {
			t.Fatal("expired confirmation remained visible")
		}
	}
}

func TestSessionDeleteConfirmationCancelsOnNavigation(t *testing.T) {
	for _, msg := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEsc},
		tea.KeyMsg{Type: tea.KeyCtrlQ},
		tea.KeyMsg{Type: tea.KeyCtrlP},
		tea.KeyMsg{Type: tea.KeyCtrlX, Paste: true},
		tea.BlurMsg{},
		tea.MouseMsg{X: 2, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		tea.MouseMsg{X: 2, Y: 4, Button: tea.MouseButtonWheelDown},
	} {
		m := deleteModel()
		m.confirmSessionDelete(time.Now())
		m.Update(msg)
		if m.deleteConfirm != nil || m.deletingID != "" || len(m.client.(*projectClient).deleted) != 0 {
			t.Fatal("navigation left a destructive shortcut armed", msg)
		}
	}
	m := deleteModel()
	m.confirmSessionDelete(time.Now())
	// Refreshing inventory should retain the ID even if its row number moves.
	m.updatePanel(listing{sessions: append([]*api.Session{{Id: "newer", ProjectId: "p", CreatedAt: 1}}, m.sessions...)})
	if m.deleteConfirm == nil || m.panelRows()[m.panelIndex].session.Id != "target" {
		t.Fatal("list reorder lost selected target")
	}
	m.updatePanel(listing{sessions: []*api.Session{{Id: "other", ProjectId: "p"}}})
	if m.deleteConfirm != nil {
		t.Fatal("removed target retained confirmation")
	}
	m = deleteModel()
	m.confirmSessionDelete(time.Now())
	m.sessions[0].RunId = "replacement"
	if m.confirmSessionDelete(time.Now()) != nil || m.deleteConfirm.run != "replacement" {
		t.Fatal("new run reused old confirmation")
	}
}

type delayedDeleteClient struct {
	api.SessionsClient
	started chan string
	release chan error
}

func (c *delayedDeleteClient) DeleteSession(ctx context.Context, id string) error {
	c.started <- id
	select {
	case err := <-c.release:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestSessionDeleteKeepsNavigatorResponsive(t *testing.T) {
	for _, failure := range []bool{false, true} {
		m := deleteModel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m.ctx = ctx
		c := &delayedDeleteClient{started: make(chan string, 1), release: make(chan error, 1)}
		m.client = c
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		select {
		case id := <-c.started:
			if id != "target" {
				t.Fatal("deleted wrong session", id)
			}
		case <-time.After(time.Second):
			t.Fatal("delete never reached the client")
		}
		if m.busy || !strings.Contains(ansi.Strip(m.panelScreen()), "Deleting session") {
			t.Fatal("deletion silently locked UI")
		}
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		if m.panelIndex != 2 || m.deletingID != "target" {
			t.Fatal("in-flight RPC blocked navigation or changed target")
		}
		m.Update(pulseTick{})
		m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
		if m.panelKey(tea.KeyMsg{Type: tea.KeyCtrlX}) != nil || m.deleteConfirm != nil {
			t.Fatal("duplicate deletion started while RPC was pending")
		}
		var err error
		if failure {
			err = errors.New("daemon unavailable")
		}
		c.release <- err
		select {
		case msg := <-done:
			m.Update(msg)
		case <-time.After(time.Second):
			t.Fatal("delete command did not finish")
		}
		if m.deletingID != "" {
			t.Fatal("RPC completion left deletion pending")
		}
		if failure {
			if len(m.sessions) != 2 || m.errorDialog == nil || !strings.Contains(m.errorDialog.text, "daemon unavailable") {
				t.Fatal("failed deletion hid session or error")
			}
		} else if len(m.sessions) != 1 || m.sessions[0].Id != "other" {
			t.Fatal("successful deletion left session in navigator")
		}
	}
}

func TestSessionDeleteDoesNotChangeAnotherConversation(t *testing.T) {
	m := deleteModel()
	m.sessions = append(m.sessions, &api.Session{Id: "last", ProjectId: "p"})
	m.selected, m.projectView = 1, false
	m.deletingID = "target"
	m.input.SetValue("draft")
	m.receiveSessionDeleted(sessionDeleted{id: "target"})
	if m.current().Id != "other" || m.projectView || m.input.Value() != "draft" {
		t.Fatal("deleting another session changed active conversation")
	}
}

func TestOldSessionDeleteKeysDoNothing(t *testing.T) {
	m := deleteModel()
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'d'}}, {Type: tea.KeyDelete}, {Type: tea.KeyRunes, Runes: []rune{'y'}}} {
		m.Update(key)
		if m.deleteConfirm != nil || m.deletingID != "" || m.busy {
			t.Fatal("old shortcut armed or blocked deletion", key)
		}
	}
}
