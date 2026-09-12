package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/muesli/termenv"
	"google.golang.org/grpc"
)

type accountViewService struct {
	resource.AccountServiceClient
	items []*resource.Account
	added *resource.AccountAddRequest
	err   error
}

func (s *accountViewService) List(context.Context, *resource.AccountListRequest, ...grpc.CallOption) (*resource.AccountListResponse, error) {
	return resource.AccountListResponse_builder{Items: s.items}.Build(), nil
}
func (s *accountViewService) Add(_ context.Context, r *resource.AccountAddRequest, _ ...grpc.CallOption) (*resource.Account, error) {
	s.added = r
	if s.err != nil {
		return nil, s.err
	}
	a := resource.Account_builder{Alias: r.GetAlias(), Name: r.GetName(), Agent: r.GetAgent()}.Build()
	s.items = append(s.items, a)
	return a, nil
}
func TestAccountViewAddAndReturn(t *testing.T) {
	m := projectModel()
	s := &accountViewService{}
	m.accountService = s
	m.createProjectSession = func(string, io.Reader, io.Writer, io.Writer) (*api.Session, error) {
		t.Fatal("created without selection")
		return nil, nil
	}
	key := func(k string) tea.Cmd {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		return cmd
	}
	cmd := key("n")
	m.Update(cmd())
	if !m.accountView || !m.accountChoosing || !strings.Contains(m.View(), "No accounts yet") {
		t.Fatal(m.View())
	}
	key("n")
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	key("work")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	key("Company")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	s.err = errors.New("duplicate alias")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if !m.accountAdding || m.accountAlias.Value() != "work" || m.notice != "duplicate alias" {
		t.Fatal("failed registration lost form")
	}
	s.err = nil
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, reload := m.Update(cmd())
	m.Update(reload())
	if m.accountAdding || s.added.GetAgent() != "codex" || s.added.GetName() != "Company" || m.accounts[0].GetAlias() != "work" {
		t.Fatal("registration failed")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.busy {
		t.Fatal("account selection did not create exec command")
	}
	m.Update(result{err: errors.New("login needed")})
	if !m.accountView || m.busy {
		t.Fatal("create failure left account view")
	}
	m.Update(accountLoggedIn{err: errors.New("login canceled")})
	if !m.accountView || m.notice != "login canceled" {
		t.Fatal("login error lost")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.accountView || !m.projectView {
		t.Fatal("did not return to project")
	}
}

func TestAccountViewValidationAndLogin(t *testing.T) {
	m := projectModel()
	m.accountService = &accountViewService{}
	m.openAccounts(false)
	m.Update(accountListing{accounts: []*resource.Account{resource.Account_builder{Alias: "main", Agent: "claude"}.Build()}})
	m.loginAccount = func(string, io.Reader, io.Writer, io.Writer) error { return nil }
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if cmd == nil || !m.busy {
		t.Fatal("login not started")
	}
	m.Update(accountLoggedIn{})
	if !m.accountView || m.busy {
		t.Fatal("login did not return")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m.accountField = 3
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.busy || !strings.Contains(m.notice, "alias") {
		t.Fatal("empty alias accepted")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.accountView || m.accountAdding {
		t.Fatal("form cancel exited view")
	}
	for _, size := range [][2]int{{40, 14}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if !strings.Contains(m.View(), "Esc back") {
			t.Fatal("navigation hidden", m.View())
		}
	}
}

func TestComposerTransparent(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	input := newComposer()
	input.SetValue("prompt\nnext")
	for _, focused := range []bool{true, false} {
		if !focused {
			input.Blur()
		}
		view := frame(input.View(), 80, focused)
		if strings.Contains(view, "48;") || strings.Contains(view, "[40m") {
			t.Fatal("composer still sets background", view)
		}
	}
}

func TestAccountSearch(t *testing.T) {
	m := projectModel()
	m.openAccounts(true)
	m.Update(accountListing{accounts: []*resource.Account{
		resource.Account_builder{Alias: "personal", Name: "Home", Agent: "claude"}.Build(),
		resource.Account_builder{Alias: "work", Name: "Company", Agent: "codex"}.Build(),
	}})
	for _, query := range []string{"work", "Company", "codex", "2"} {
		m.accountSearch.SetValue(query)
		if got := m.accountChoices(); len(got) != 1 || got[0].GetAlias() != "work" {
			t.Fatal(query, got)
		}
	}
	m.accountSearch.SetValue("missing")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.busy || !strings.Contains(m.View(), "No matching accounts") {
		t.Fatal("selected absent account")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if m.accountView {
		t.Fatal("Ctrl+Q did not return to project")
	}
}

func TestCompactCostAndMetricSlots(t *testing.T) {
	for _, n := range []float64{0, 1, 10, 100, 999.9, 1000, 1234, 999999, 1e6} {
		value, scale := compactMetric(n)
		if len(value) != 3 || len(scale) != 1 {
			t.Fatalf("bad slot %q %q", value, scale)
		}
	}
	for n, want := range map[float64]string{0: "$  0 ", 0.1: "$0.1 ", 0.0123: "¢1.2 ", 0.0001: "¢<.1 "} {
		if got := costMetric(n); got != want {
			t.Fatalf("cost %v = %q, want %q", n, got, want)
		}
	}
}
