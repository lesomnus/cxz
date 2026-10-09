package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/engine"
	"google.golang.org/grpc"
)

// calls records what the page asked for, one entry per button and one for each
// refresh. The refresh is three calls on the wire now -- the engine, the
// budgets and the build -- but it is one question the page asks, so it is one
// entry here.
type settingsClient struct {
	api.SessionsClient
	calls []string
	fail  bool
	state string
}

func (c *settingsClient) GetEngineInfo(_ context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.EngineInfo, error) {
	c.calls = append(c.calls, "info")
	if c.fail {
		return nil, errors.New("unsupported manager")
	}
	state := c.state
	if state == "" {
		state = "running"
	}
	return &api.EngineInfo{Mode: "dind", State: state, Image: "docker:29-dind", BuildCache: "20MB", Reclaimable: "10MB"}, nil
}
func (c *settingsClient) GetHistoryPolicy(_ context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.HistoryPolicy, error) {
	return &api.HistoryPolicy{}, nil
}
func (c *settingsClient) GetInstallationVersion(_ context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.InstallationVersion, error) {
	return &api.InstallationVersion{}, nil
}
func (c *settingsClient) StartEngine(_ context.Context, _ *api.StartEngineInput, _ ...grpc.CallOption) (*api.EngineReply, error) {
	c.calls = append(c.calls, "up")
	if c.fail {
		return nil, errors.New("unsupported manager")
	}
	c.state = "running"
	return &api.EngineReply{Status: "Done"}, nil
}
func (c *settingsClient) StopEngine(_ context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.EngineReply, error) {
	c.calls = append(c.calls, "down")
	if c.fail {
		return nil, errors.New("unsupported manager")
	}
	c.state = "not running"
	return &api.EngineReply{Status: "Done"}, nil
}
func (c *settingsClient) PruneEngine(_ context.Context, _ *api.Empty, _ ...grpc.CallOption) (*api.EngineReply, error) {
	c.calls = append(c.calls, "prune")
	if c.fail {
		return nil, errors.New("unsupported manager")
	}
	return &api.EngineReply{Status: "Done"}, nil
}
func TestSettingsShortcutAndMaintenance(t *testing.T) {
	m := conversationModel()
	c := &settingsClient{}
	m.client = c
	m.input.SetValue("draft")
	m.input.Focus()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF19})
	if cmd == nil || m.settingsPage == nil || m.input.Focused() {
		t.Fatal("settings not focused")
	}
	m.Update(cmd())
	p := m.settingsPage
	if !p.loaded || p.info.BuildCache != "20MB" {
		t.Fatal(p)
	}
	p.selected = 2
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
	if len(c.calls) != 3 || c.calls[1] != "prune" {
		t.Fatal(c.calls)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF19})
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
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF19})
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

func TestSettingsEngineToggle(t *testing.T) {
	m := conversationModel()
	c := &settingsClient{state: "not running"}
	m.client = c
	m.Update(m.openSettings()())
	p := m.settingsPage
	p.selected = 1
	if text := ansi.Strip(m.View()); !strings.Contains(text, "[ Activate ]") || strings.Contains(text, "Deactivate") {
		t.Fatal(text)
	}
	cmd := m.activateSetting()
	if cmd == nil {
		t.Fatal("Activate did not start engine")
	}
	if strings.Contains(ansi.Strip(m.View()), "› [") {
		t.Fatal("busy buttons show selection")
	}
	m.Update(cmd())
	// Activate applies the configuration already saved, so it carries none.
	if c.calls[1] != "up" {
		t.Fatal(c.calls)
	}
	if text := ansi.Strip(m.View()); !strings.Contains(text, "[ Deactivate ]") || strings.Contains(text, "[ Activate ]") {
		t.Fatal(text)
	}
	if cmd = m.activateSetting(); cmd != nil || p.confirm != "down" || p.confirmYes {
		t.Fatal("deactivation lacks confirmation")
	}
	m.settingsKey(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(m.settingsKey(tea.KeyMsg{Type: tea.KeyEnter})())
	if c.calls[3] != "down" || p.info.State != "not running" {
		t.Fatal(c.calls, p.info)
	}
	if text := ansi.Strip(m.View()); !strings.Contains(text, "[ Activate ]") {
		t.Fatal(text)
	}
}

func TestSettingsSkipDisabledButtons(t *testing.T) {
	m := conversationModel()
	m.settingsPage = &settingsPage{loaded: true, info: settingsInfo{Info: engine.Info{Mode: "dind", State: "not running"}}}
	p := m.settingsPage
	for _, key := range []tea.KeyType{tea.KeyDown, tea.KeyTab} {
		p.selected = 1
		m.settingsKey(tea.KeyMsg{Type: key})
		if p.selected != 3 {
			t.Fatal("did not skip disabled cache action", p.selected)
		}
	}
	for _, key := range []tea.KeyType{tea.KeyUp, tea.KeyShiftTab} {
		p.selected = 3
		m.settingsKey(tea.KeyMsg{Type: key})
		if p.selected != 1 {
			t.Fatal("did not skip disabled cache action", p.selected)
		}
	}
	m.settingsMouse(tea.MouseMsg{X: 3, Y: settingsActionRow + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if p.selected != 1 || p.confirm != "" {
		t.Fatal("disabled mouse click selected action")
	}
	p.info.State = "running"
	p.selected = 2
	m.receiveSettings(settingsResult{page: p, request: p.request, action: "info", info: settingsInfo{Info: engine.Info{Mode: "off", State: "not running"}}})
	if p.selected != 3 {
		t.Fatal("focus stayed on disabled action")
	}
	m.settingsKey(tea.KeyMsg{Type: tea.KeyDown})
	if p.selected != 9 {
		t.Fatal("MCP settings must remain available while Docker is stopped")
	}
	m.settingsKey(tea.KeyMsg{Type: tea.KeyDown})
	if p.selected != 10 {
		t.Fatal("AI settings must remain available while Docker is stopped")
	}
	m.settingsKey(tea.KeyMsg{Type: tea.KeyDown})
	if p.selected != 0 {
		t.Fatal("did not wrap to refresh")
	}
	// Local recording remains accessible even if Docker is loading/unavailable.
	p.loading = true
	m.settingsKey(tea.KeyMsg{Type: tea.KeyUp})
	if p.selected != 3 || !m.settingsEnabled(3) {
		t.Fatal("recording depends on Docker")
	}
	m.recordingSaving = true
	if strings.Contains(ansi.Strip(m.View()), "› [") {
		t.Fatal("busy buttons show selection")
	}
}
