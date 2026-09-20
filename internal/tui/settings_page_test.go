package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/engine"
	"google.golang.org/grpc"
)

type settingsClient struct {
	api.SessionsClient
	calls []*api.DockerInput
	fail  bool
}

func (c *settingsClient) Docker(_ context.Context, r *api.DockerInput, _ ...grpc.CallOption) (*api.Receipt, error) {
	c.calls = append(c.calls, r)
	if c.fail {
		return nil, errors.New("unsupported manager")
	}
	if r.Action == "info" {
		b, _ := json.Marshal(engine.Info{Mode: "dind", State: "running", Image: "docker:29-dind", BuildCache: "20MB", Reclaimable: "10MB"})
		return &api.Receipt{Status: string(b)}, nil
	}
	return &api.Receipt{Status: "Done"}, nil
}
func TestSettingsShortcutAndMaintenance(t *testing.T) {
	m := conversationModel()
	c := &settingsClient{}
	m.client = c
	m.input.SetValue("draft")
	m.input.Focus()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if cmd == nil || m.settingsPage == nil || m.input.Focused() {
		t.Fatal("settings not focused")
	}
	m.Update(cmd())
	p := m.settingsPage
	if !p.loaded || p.info.BuildCache != "20MB" {
		t.Fatal(p)
	}
	p.selected = 3
	if cmd = m.activateSetting(); cmd != nil || p.confirm != "prune" || p.confirmYes {
		t.Fatal("missing cancel-first confirmation")
	}
	m.settingsKey(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	if p.confirm != "prune" {
		t.Fatal("paste activated button")
	}
	m.settingsKey(tea.KeyMsg{Type: tea.KeyEnter})
	if p.confirm != "" || len(c.calls) != 1 {
		t.Fatal("cancel mutated engine")
	}
	m.activateSetting()
	m.settingsKey(tea.KeyMsg{Type: tea.KeyTab})
	cmd = m.settingsKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.settingsRequest("info") != nil {
		t.Fatal("request concurrency")
	}
	m.Update(cmd())
	if len(c.calls) != 3 || c.calls[1].Action != "prune" || len(c.calls[1].Spec) != 0 {
		t.Fatal(c.calls)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.settingsPage != nil || !m.input.Focused() || m.input.Value() != "draft" {
		t.Fatal("draft/focus lost")
	}
	m.receiveSettings(settingsResult{page: p, request: p.request}) // late closed-page response
}

func TestSettingsErrorsResizeAndProjectEntry(t *testing.T) {
	m := conversationModel()
	c := &settingsClient{fail: true}
	m.client = c
	m.projectView = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m.Update(cmd())
	p := m.settingsPage
	if !strings.Contains(p.statusError, "unsupported manager") {
		t.Fatal(p.statusError)
	}
	c.fail = false
	m.Update(m.settingsRequest("info")())
	if p.statusError != "" || !p.loaded {
		t.Fatal("stale error", p)
	}
	p.message = strings.Repeat("long error detail ", 50) + "END"
	for _, width := range []int{30, 80, 180} {
		m.terminalWidth = width
		m.height = 20
		screen := m.View()
		for _, line := range strings.Split(screen, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("overflow at %d: %q", width, line)
			}
		}
		m.settingsKey(tea.KeyMsg{Type: tea.KeyEnd})
		if !strings.Contains(ansi.Strip(m.View()), "END") {
			t.Fatal("error not scrollable")
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.projectView {
		t.Fatal("project navigation lost")
	}
}
