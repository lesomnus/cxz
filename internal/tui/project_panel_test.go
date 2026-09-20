package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"google.golang.org/grpc"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
)

func TestPanelLongListAndDeletedProjects(t *testing.T) {
	m := panelModel()
	projects := []*api.Project{}
	for i := 0; i < 80; i++ {
		projects = append(projects, &api.Project{Id: fmt.Sprint(i), Name: fmt.Sprintf("Project %02d", i)})
	}
	m.updatePanel(listing{projects: projects, projectsLoaded: true})
	m.focusPanel()
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.panelIndex != 79 || !strings.Contains(ansi.Strip(m.panelScreen()), "Project 79") {
		t.Fatal("last project inaccessible")
	}
	m.updatePanel(listing{projectsLoaded: true})
	if len(m.panelRows()) != 0 || m.panelIndex != 0 {
		t.Fatal("deleted projects retained")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func panelModel() *model {
	m := conversationModel()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	m.updatePanel(listing{projects: []*api.Project{m.project, {Id: "other", Name: "Other project"}}, sessions: []*api.Session{m.current(), {Id: "other-session", Alias: "maple", ProjectId: "other", Agent: "codex", RunId: "other-run"}}})
	return m
}

func TestWideViewCapAndBlankRemainder(t *testing.T) {
	for _, width := range []int{80, 133, 150, 170, 171, 200, 300} {
		m := panelModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		if m.width != min(width, 133) || m.panelVisible() != (width >= 171) {
			t.Fatal("wrong breakpoint", width)
		}
		for _, view := range []string{"session", "project", "accounts"} {
			m.projectView = view == "project"
			m.accountView = view == "accounts"
			lines := strings.Split(m.View(), "\n")
			if len(lines) != 30 {
				t.Fatal("wrong height", width, view)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != width {
					t.Fatal("wrong screen width", width, view)
				}
				if strings.TrimSpace(ansi.Strip(ansi.Cut(line, m.contentOffset()+m.width, width))) != "" {
					t.Fatal("right remainder not blank")
				}
			}
		}
	}
}

func TestProjectPanelFocusAndSessionSwitch(t *testing.T) {
	m := panelModel()
	m.input.SetValue("keep my draft")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.panelFocus {
		t.Fatal("Tab entered project panel")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !m.panelFocus || m.projectView {
		t.Fatal("Ctrl+Q must focus panel without leaving session view")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !m.panelFocus {
		t.Fatal("Tab must not leave panel")
	}
	for i, r := range m.panelRows() {
		if r.session != nil && r.session.Id == "other-session" {
			m.panelIndex = i
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.current().Id != "other-session" || m.project.Id != "other" || m.panelFocus || m.projectView {
		t.Fatal("did not switch project/session")
	}
	if m.drafts["s"] != "keep my draft" || m.input.Value() != "" {
		t.Fatal("draft leaked or lost")
	}
	// An in-flight refresh from the old project must not change the destination.
	m.Update(listing{project: &api.Project{Id: "p"}, sessions: m.allSessions})
	if m.project.Id != "other" || m.current().Id != "other-session" {
		t.Fatal("stale refresh undid switch")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 30})
	if !m.panelFocus || m.input.Focused() || !strings.Contains(ansi.Strip(m.View()), "Projects") {
		t.Fatal("narrow navigation lost focus or project list")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if m.panelFocus || m.projectView || !m.input.Focused() {
		t.Fatal("narrow Ctrl+Q did not return to conversation")
	}
}

func TestWorkflowReceivesSelectedProject(t *testing.T) {
	m := panelModel()
	m.project = &api.Project{Id: "other"}
	m.createProjectSession = func(_ context.Context, project, account string, _ io.Reader, _ io.Writer, _ io.Writer) (*api.Session, error) {
		if project != "other" || account != "main" {
			t.Error("workflow targeted original project")
		}
		return &api.Session{Id: "new"}, nil
	}
	cmd := m.startAccountWorkflow("main", "codex", "", true)
	defer m.workflow.close()
	batch := cmd().(tea.BatchMsg)
	if done := batch[0]().(workflowDone); done.err != nil {
		t.Fatal(done.err)
	}
}

func TestProjectNavigatorPreservesFocusAcrossResponsiveLayouts(t *testing.T) {
	m := panelModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.input.SetValue("unfinished draft")
	canceled := false
	m.watchCancel = func() { canceled = true }
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !m.panelFocus || m.projectView || canceled {
		t.Fatal("opening navigator left the conversation")
	}
	for i, r := range m.panelRows() {
		if r.session != nil && r.session.Id == "other-session" {
			m.panelIndex = i
		}
	}
	selected := m.panelRows()[m.panelIndex].key()
	for _, width := range []int{80, 200, 150, 171, 80} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		if !m.panelFocus || m.input.Focused() || m.panelRows()[m.panelIndex].key() != selected || m.current().Id != "s" || m.input.Value() != "unfinished draft" {
			t.Fatal("resize lost navigation state", width)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "Projects") || !strings.Contains(view, "maple") {
			t.Fatal("missing shared navigator", width)
		}
		if width < 171 && strings.Contains(view, "unfinished draft") {
			t.Fatal("composer visible under full-screen navigator")
		}
		for _, row := range strings.Split(view, "\n") {
			if ansi.StringWidth(row) != width {
				t.Fatal("layout width", width, ansi.StringWidth(row))
			}
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.panelFocus || !m.input.Focused() || m.current().Id != "s" || m.input.Value() != "unfinished draft" {
		t.Fatal("return did not preserve conversation")
	}
}

func TestNarrowNavigatorOpensOtherProjectSession(t *testing.T) {
	m := panelModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.input.SetValue("saved draft")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	for i, r := range m.panelRows() {
		if r.session != nil && r.session.Id == "other-session" {
			m.panelIndex = i
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.panelFocus || m.projectView || m.current().Id != "other-session" || m.drafts["s"] != "saved draft" {
		t.Fatal("narrow selection did not open destination")
	}
}

func TestNavigatorEntryFocusesRequestedProject(t *testing.T) {
	m := conversationModel()
	target := &api.Project{Id: "target", Name: "Z target"}
	m.initializeNavigation(target, "")
	m.updatePanel(listing{projects: []*api.Project{{Id: "first", Name: "A first"}, target}, projectsLoaded: true, sessions: []*api.Session{{Id: "child", ProjectId: "target"}}})
	if !m.panelFocus || !m.projectView || m.panelRows()[m.panelIndex].key() != "p:target" {
		t.Fatal("startup did not select workspace project")
	}
	m.panelKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.panelFocus || m.panelRows()[m.panelIndex].key() != "s:child" {
		t.Fatal("project opened an obsolete screen")
	}
	m.panelKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.projectView || m.panelFocus || m.current().Id != "child" {
		t.Fatal("session not opened")
	}
}

type navigatorClient struct {
	api.SessionsClient
	stopped, deleted string
	run              string
}

func (c *navigatorClient) Stop(_ context.Context, r *api.Control, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.stopped = r.SessionId
	c.run = r.RunId
	return &api.Receipt{}, nil
}
func (c *navigatorClient) DeleteSession(_ context.Context, id string) error {
	c.deleted = id
	return nil
}

func TestNavigatorActionsUseSelectedSessionAndProject(t *testing.T) {
	for _, width := range []int{80, 200} {
		m := panelModel()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m.focusPanel()
		c := &navigatorClient{}
		m.client = c
		for i, r := range m.panelRows() {
			if r.session != nil && r.session.Id == "other-session" {
				m.panelIndex = i
			}
		}
		stop := m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		stop()
		if c.stopped != "other-session" || c.run != "other-run" || m.current().Id != "s" {
			t.Fatal("stop targeted viewed session")
		}
		m.busy = false
		m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		m.panelIndex = 0
		confirm := m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		confirm()
		if c.deleted != "other-session" {
			t.Fatal("delete target drifted")
		}
		m.busy = false
		for i, r := range m.panelRows() {
			if r.project.Id == "other" && r.session == nil {
				m.panelIndex = i
			}
		}
		m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		if !m.accountView || m.project.Id != "other" {
			t.Fatal("accounts targeted original project")
		}
		m.accountKey(tea.KeyMsg{Type: tea.KeyEsc})
		if !m.panelFocus || m.accountView {
			t.Fatal("accounts did not return to navigator")
		}
		m.createProjectSession = func(_ context.Context, project, account string, _ io.Reader, _ io.Writer, _ io.Writer) (*api.Session, error) {
			if project != "other" {
				t.Error("creation targeted original project")
			}
			return &api.Session{Id: "new"}, nil
		}
		m.panelKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		if !m.accountChoosing || !m.accountView || m.project.Id != "other" {
			t.Fatal("new session did not open account choice")
		}
		cmd := m.startAccountWorkflow("main", "codex", "", true)
		batch := cmd().(tea.BatchMsg)
		if done := batch[0]().(workflowDone); done.err != nil {
			t.Fatal(done.err)
		}
		m.workflow.close()
	}
}

func TestProjectNavigatorHasBackgroundAndOnlyFocusedSideBottomRule(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(profile)
	m := panelModel()
	for _, width := range []int{80, 200} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		for _, focus := range []bool{false, true} {
			m.panelFocus = focus
			rendered := m.panelScreen()
			plain := ansi.Strip(rendered)
			if strings.ContainsAny(plain, "╭╮╰╯│") {
				t.Fatal("outer border remains")
			}
			if strings.Contains(plain, "─") != (width == 200 && focus) {
				t.Fatal("incorrect focus rule")
			}
			if !strings.Contains(rendered, "48;2;16;32;43") {
				t.Fatal("background missing")
			}
		}
	}
}
