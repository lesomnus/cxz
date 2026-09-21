package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

type sessionCreationClient struct {
	api.SessionsClient
	request *api.ProjectRequest
	err     error
	remote  bool
}

func (c *sessionCreationClient) Open(ctx context.Context, r *api.ProjectRequest, _ ...grpc.CallOption) (*api.Session, error) {
	c.request, c.remote = r, transport.IsRemote(ctx)
	return &api.Session{Id: "work::new", ProjectId: r.Workspace, Agent: r.Agent, Account: r.Account}, c.err
}

func TestNavigatorNewSessionWithoutLocalCreator(t *testing.T) {
	for _, width := range []int{69, 110, 200} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := conversationModel()
			m.ctx = settings.With(transport.WithRemote(m.ctx), settings.Config{CodexModel: "saved-model"})
			m.project.Id = "work::p"
			m.current().ProjectId = m.project.Id
			m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
			m.input.SetValue("keep my draft")
			m.events["s"] = []*api.Event{{Kind: "assistant", Text: "old conversation must stay hidden"}}
			client := &sessionCreationClient{err: errors.New("session preparation failed")}
			m.client = client
			m.accountService = &accountViewService{items: []*resource.Account{
				resource.Account_builder{Alias: "personal", Agent: "claude"}.Build(),
				resource.Account_builder{Alias: "work", Agent: "codex"}.Build(),
			}}
			m.focusPanel()
			_, load := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
			if !m.accountView || !m.accountChoosing || m.creating || load == nil {
				t.Fatal("new session fell back to the chat composer")
			}
			m.Update(load())
			view := ansi.Strip(m.View())
			if !strings.Contains(view, "Choose an account") || strings.Contains(view, "old conversation") || m.drafts["s"] != "keep my draft" {
				t.Fatal("creation screen or draft lost", view)
			}
			m.Update(tea.KeyMsg{Type: tea.KeyDown})
			_, create := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if create == nil || m.workflow == nil || !strings.Contains(ansi.Strip(m.View()), "Preparing agent session") {
				t.Fatal("no creation progress screen")
			}
			m.Update(create().(tea.BatchMsg)[0]())
			if client.request.Workspace != "work::p" || client.request.Account != "work" || client.request.Agent != "codex" || client.request.Model != "saved-model" || !client.request.NewSession || client.request.ClientId == "" || !client.remote {
				t.Fatal("wrong creation target", client.request)
			}
			if !m.accountView || m.busy || m.workflow != nil || !strings.Contains(ansi.Strip(m.View()), "session preparation failed") {
				t.Fatal("creation error escaped the account view", m.View())
			}
			client.err = nil
			_, create = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m.Update(create().(tea.BatchMsg)[0]())
			if m.accountView || m.projectView || m.busy || m.wantID != "work::new" || !m.input.Focused() {
				t.Fatal("successful creation did not open the new session")
			}
		})
	}
}

func TestNewSessionAccountLoadFailureAndCancel(t *testing.T) {
	m := panelModel()
	m.accountService = nil
	m.input.SetValue("preserved")
	m.focusPanel()
	_, load := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m.Update(load())
	if !m.accountView || m.accountLoading || !strings.Contains(m.View(), "Account service unavailable") {
		t.Fatal("account error shown in chat", m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.accountView || !m.panelFocus || m.drafts["s"] != "preserved" {
		t.Fatal("cancel did not return to navigator or lost draft")
	}
}

func TestConversationNewSessionUsesAccountPicker(t *testing.T) {
	m := conversationModel()
	m.input.SetValue("preserved")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if !m.accountView || !m.accountChoosing || m.creating || m.drafts["s"] != "preserved" {
		t.Fatal("conversation shortcut still uses legacy workspace input")
	}
}
