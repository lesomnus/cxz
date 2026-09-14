package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

func TestWorkflowProcessExitDoesNotWaitForMoreInput(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			m := projectModel()
			m.createProjectSession = func(ctx context.Context, _ string, _ string, in io.Reader, out, errOut io.Writer) (*api.Session, error) {
				script := `printf 'Login successful.\n'`
				if provider == "claude" {
					script = `IFS= read -r code; test "$code" = 'fixture#state' || exit 1; printf 'Login successful.\n'`
				}
				cmd := exec.CommandContext(ctx, "sh", "-c", script)
				cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, errOut
				if err := cmd.Run(); err != nil {
					return nil, err
				}
				return &api.Session{Id: "created"}, nil
			}
			batch := m.startAccountWorkflow("main", provider, "", true)().(tea.BatchMsg)
			f := m.workflow
			defer f.close()
			done := make(chan tea.Msg, 1)
			go func() { done <- batch[0]() }()
			if provider == "claude" {
				f.input.SetValue("fixture#state")
				m.Update(m.workflowKey(tea.KeyMsg{Type: tea.KeyEnter})())
			}
			select {
			case msg := <-done:
				if result := msg.(workflowDone); result.err != nil {
					t.Fatal(result.err)
				}
				m.Update(msg)
				if m.workflow != nil || m.wantID != "created" {
					t.Fatal("login did not attach session")
				}
			case <-time.After(time.Second):
				f.close()
				<-done
				t.Fatal("exited login process blocked waiting for input EOF")
			}
		})
	}
}

func TestWorkflowStaysInAppAndPipesSecret(t *testing.T) {
	m := projectModel()
	m.accountView = true
	m.createProjectSession = func(ctx context.Context, _ string, alias string, in io.Reader, out, errOut io.Writer) (*api.Session, error) {
		if alias != "main" {
			return nil, fmt.Errorf("wrong account")
		}
		fmt.Fprint(out, "\x1b[?1049lVisit https://example.invalid/login\nPaste code: ")
		code, err := bufio.NewReader(in).ReadString('\n')
		if err != nil {
			return nil, err
		}
		if code != "secret#state\n" {
			return nil, fmt.Errorf("wrong code")
		}
		return &api.Session{Id: "created"}, nil
	}
	cmd := m.startAccountWorkflow("main", "claude", "", true)
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("workflow must be asynchronous, not tea.Exec")
	}
	defer m.workflow.close()
	done := make(chan tea.Msg, 1)
	go func() { done <- batch[0]() }()
	m.Update(batch[1]())
	if view := m.View(); !strings.Contains(view, "https://example.invalid/login") || strings.Contains(view, "\x1b[?1049l") || !m.accountView {
		t.Fatal(view)
	}
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if len(strings.Split(m.View(), "\n")) != size[1] {
			t.Fatal("workflow did not resize")
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("discard")})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret#state")})
	if view := ansi.Strip(m.View()); strings.Contains(view, "secret") || !strings.Contains(view, "[***]") || !strings.Contains(view, "012") {
		t.Fatal(view)
	}
	_, write := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(write())
	select {
	case msg := <-done:
		m.Update(msg)
	case <-time.After(time.Second):
		t.Fatal("workflow blocked")
	}
	if m.workflow != nil || m.busy || m.wantID != "created" || m.accountView {
		t.Fatal("successful create did not attach")
	}
}

func TestWorkflowCancelReturnsToAccounts(t *testing.T) {
	m := projectModel()
	m.accountView = true
	m.loginAccount = func(ctx context.Context, _ string, _ string, _ string, _ io.Reader, _ io.Writer, _ io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	}
	batch := m.startAccountWorkflow("work", "codex", "", false)().(tea.BatchMsg)
	done := make(chan tea.Msg, 1)
	go func() { done <- batch[0]() }()
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	select {
	case msg := <-done:
		m.Update(msg)
	case <-time.After(time.Second):
		t.Fatal("cancel blocked")
	}
	if m.workflow != nil || m.busy || !m.accountView || !strings.Contains(m.notice, "canceled") {
		t.Fatal("cancel lost account view")
	}
}

func TestClaudeAccountLoginSelectsScopedSession(t *testing.T) {
	m := projectModel()
	m.accountView = true
	m.accounts = []*resource.Account{resource.Account_builder{Alias: "main", Agent: "claude"}.Build()}
	m.sessions = []*api.Session{
		{Id: "own", Alias: "own", Account: "main", Agent: "claude", ProjectId: m.project.Id, State: "idle"},
		{Id: "other-account", Account: "other", Agent: "claude", ProjectId: m.project.Id},
		{Id: "other-project", Account: "main", Agent: "claude", ProjectId: "elsewhere"},
	}
	got := ""
	m.loginAccount = func(_ context.Context, _ string, _ string, id string, _ io.Reader, _ io.Writer, _ io.Writer) error {
		got = id
		return nil
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !m.loginChoosing || len(m.loginSessions()) != 1 || m.workflow != nil {
		t.Fatal("login scope incorrect")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.workflow != nil || !strings.Contains(m.notice, "Stop this session") {
		t.Fatal("live credentials could be replaced")
	}
	m.sessions[0].State = "stopped"
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	batch := cmd().(tea.BatchMsg)
	m.Update(batch[0]())
	if got != "own" || !m.accountView {
		t.Fatal("wrong login target")
	}
}
