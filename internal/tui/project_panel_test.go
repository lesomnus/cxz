package tui

import (
	"context"
	"fmt"
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
	if m.panelFocus || !m.input.Focused() {
		t.Fatal("hidden panel retained focus")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if !m.projectView {
		t.Fatal("narrow Ctrl+Q must retain project view navigation")
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
