package tui

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/engine"
)

type settingsPage struct {
	info                                engine.Info
	loaded, loading, busy, inputFocused bool
	selected, offset                    int
	request                             uint64
	lastRefresh                         time.Time
	message, statusError                string
	confirm                             string
	confirmYes                          bool
}
type settingsResult struct {
	page         *settingsPage
	request      uint64
	action       string
	info         engine.Info
	text         string
	err, infoErr error
}

var settingsActions = []struct{ label, action string }{
	{"Refresh", "info"}, {"Activate", "up"}, {"Clear unused build cache", "prune"},
}

const settingsActionRow = 7

func (m *model) openSettings() tea.Cmd {
	m.settingsPage = &settingsPage{inputFocused: m.input.Focused()}
	m.input.Blur()
	return m.settingsRequest("info")
}
func (m *model) settingsRequest(action string) tea.Cmd {
	p := m.settingsPage
	if p == nil || p.loading || p.busy {
		return nil
	}
	p.request++
	request := p.request
	p.loading = true
	p.busy = action != "info"
	p.lastRefresh = time.Now()
	if action != "info" {
		p.message = "Working…"
	}
	client, ctx := m.client, m.ctx
	return func() tea.Msg {
		timeout := 20 * time.Second
		if action != "info" {
			timeout = 3 * time.Minute
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		result := settingsResult{page: p, request: request, action: action}
		if action != "info" {
			out, err := client.Docker(ctx, &api.DockerInput{Action: action})
			result.err = err
			if out != nil {
				result.text = out.Status
			}
		}
		out, err := client.Docker(ctx, &api.DockerInput{Action: "info"})
		result.infoErr = err
		if err == nil {
			result.infoErr = json.Unmarshal([]byte(out.Status), &result.info)
		}
		return result
	}
}
func (m *model) receiveSettings(r settingsResult) {
	p := m.settingsPage
	if p == nil || p != r.page || p.request != r.request {
		return
	}
	p.loading = false
	p.busy = false
	p.lastRefresh = time.Now()
	p.statusError = ""
	if r.infoErr == nil {
		p.info = r.info
		p.loaded = true
	}
	if r.action != "info" {
		p.message = r.text
	}
	if r.err != nil {
		p.message = "Action failed: " + r.err.Error()
	}
	if r.infoErr != nil {
		p.statusError = "Status unavailable: " + r.infoErr.Error() + "\nUpdate the host manager if this operation is unavailable."
	}
	if !m.settingsEnabled(p.selected) {
		m.moveSetting(1)
	}
}
func (m *model) pollSettings() tea.Cmd {
	p := m.settingsPage
	if p == nil || p.confirm != "" || time.Since(p.lastRefresh) < 10*time.Second {
		return nil
	}
	return m.settingsRequest("info")
}

// Resolve the toggle from the latest reported engine state.
func (m *model) settingsAction(index int) (string, string) {
	if index == 1 && m.settingsPage.loaded && m.settingsPage.info.State == "running" {
		return "Deactivate", "down"
	}
	a := settingsActions[index]
	return a.label, a.action
}

func (m *model) moveSetting(direction int) {
	p := m.settingsPage
	for step := 1; step <= len(settingsActions); step++ {
		index := (p.selected + direction*step + len(settingsActions)) % len(settingsActions)
		if m.settingsEnabled(index) {
			p.selected = index
			m.revealSetting()
			return
		}
	}
}

func (m *model) settingsEnabled(index int) bool {
	p := m.settingsPage
	if p == nil || index < 0 || index >= len(settingsActions) || p.loading || p.busy {
		return false
	}
	if index == 0 {
		return true
	}
	if !p.loaded || p.statusError != "" {
		return false
	}
	if index == 1 {
		return p.info.State == "running" || p.info.Mode == "dind"
	}
	return p.info.State == "running"
}
func (m *model) activateSetting() tea.Cmd {
	p := m.settingsPage
	if !m.settingsEnabled(p.selected) {
		return nil
	}
	_, action := m.settingsAction(p.selected)
	if action == "down" || action == "prune" {
		p.confirm = action
		p.confirmYes = false
		return nil
	}
	return m.settingsRequest(action)
}
func (m *model) settingsKey(k tea.KeyMsg) tea.Cmd {
	p := m.settingsPage
	if k.Paste {
		return nil
	}
	if k.String() == "ctrl+c" {
		return tea.Quit
	}
	if p.confirm != "" {
		switch k.String() {
		case "esc":
			p.confirm = ""
		case "left", "right", "tab", "shift+tab":
			p.confirmYes = !p.confirmYes
		case "enter":
			action, yes := p.confirm, p.confirmYes
			p.confirm = ""
			if yes {
				return m.settingsRequest(action)
			}
		}
		return nil
	}
	switch k.String() {
	case "esc", "ctrl+p":
		m.settingsPage = nil
		if p.inputFocused {
			return m.input.Focus()
		}
		return nil
	case "up", "shift+tab":
		m.moveSetting(-1)
	case "down", "tab":
		m.moveSetting(1)
	case "enter":
		return m.activateSetting()
	case "r":
		return m.settingsRequest("info")
	case "pgdown":
		p.offset += max(1, m.height-4)
	case "pgup":
		p.offset = max(0, p.offset-max(1, m.height-4))
	case "home":
		p.offset = 0
	case "end":
		p.offset = 1 << 20
	}
	return nil
}
func (m *model) revealSetting() {
	p := m.settingsPage
	row := settingsActionRow + p.selected
	p.offset = max(0, min(p.offset, row))
	p.offset = max(p.offset, row-max(1, m.height-2)+1)
}
func (m *model) settingsMouse(v tea.MouseMsg) tea.Cmd {
	p := m.settingsPage
	if p.confirm != "" {
		if v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
			_, cancelRow, yesRow := m.settingsConfirmation()
			width := max(1, m.settingsWidth()-4)
			if v.X >= 2 && v.X < width+2 {
				if v.Y == cancelRow {
					p.confirm = ""
				}
				if v.Y == yesRow {
					action := p.confirm
					p.confirm = ""
					return m.settingsRequest(action)
				}
			}
		}
		return nil
	}
	switch v.Button {
	case tea.MouseButtonWheelUp:
		p.offset = max(0, p.offset-3)
	case tea.MouseButtonWheelDown:
		p.offset += 3
	case tea.MouseButtonLeft:
		if v.Action != tea.MouseActionPress || v.Y >= m.height-1 || v.X < 2 || v.X >= m.settingsWidth()-2 {
			return nil
		}
		index := v.Y + p.offset - settingsActionRow
		if m.settingsEnabled(index) {
			p.selected = index
			return m.activateSetting()
		}
	}
	return nil
}
func (m *model) settingsWidth() int {
	if m.terminalWidth > 0 {
		return m.terminalWidth
	}
	return max(1, m.width)
}
func (m *model) settingsConfirmation() ([]string, int, int) {
	p := m.settingsPage
	width := max(1, m.settingsWidth()-4)
	text := map[string]string{"down": "Deactivate Docker for all projects? Images and volumes will be retained.", "prune": "Remove unused build cache from the shared Docker engine? Images and volumes will be retained."}[p.confirm]
	lines := []string{"", accent.Bold(true).Render("Confirm action"), ""}
	lines = append(lines, strings.Split(ansi.Hardwrap(text, width, true), "\n")...)
	lines = append(lines, "")
	cancelRow := len(lines)
	yesRow := cancelRow + 1
	for i, label := range []string{"Cancel", "Confirm"} {
		line := "  [ " + label + " ]"
		if (i == 1) == p.confirmYes {
			line = accent.Bold(true).Render("› [ " + label + " ]")
		}
		lines = append(lines, panelBackground(line+strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))))
	}
	lines = append(lines, "", "←/→ choose · Enter select · Esc cancel")
	return lines, cancelRow, yesRow
}
func (m *model) settingsScreen() string {
	p := m.settingsPage
	width := m.settingsWidth()
	inner := max(1, width-4)
	if p.confirm != "" {
		lines, _, _ := m.settingsConfirmation()
		return screen(indentBlock(strings.Join(lines, "\n")), width, m.height)
	}
	state := "Loading…"
	mode := "—"
	if p.statusError != "" {
		state = "Unavailable"
	}
	image, endpoint, cache := "—", "—", "—"
	if p.loaded {
		state = p.info.State
		if p.statusError != "" {
			state += " (last known)"
		}
		if p.info.Health != "" {
			state += " · " + p.info.Health
		}
		mode = p.info.Mode
		image = p.info.Image
		endpoint = p.info.Endpoint
		if p.info.BuildCache != "" {
			cache = p.info.BuildCache + " · reclaimable " + p.info.Reclaimable
		}
	}
	lines := []string{"", accent.Bold(true).Render("Settings · shared Docker"), "Mode: " + mode + "   Status: " + state, "Image: " + image, "Endpoint: " + endpoint, "Build cache: " + cache, ""}
	for i := range settingsActions {
		name, _ := m.settingsAction(i)
		label := "  [ " + name + " ]"
		if p.selected == i && m.settingsEnabled(i) {
			label = "› [ " + name + " ]"
		}
		line := clip(label, inner)
		if !m.settingsEnabled(i) {
			line = muted.Render(line)
		} else if p.selected == i {
			line = accent.Bold(true).Render(line)
		}
		lines = append(lines, panelBackground(line+strings.Repeat(" ", max(0, inner-ansi.StringWidth(line)))))
	}
	lines = append(lines, accent.Render(strings.Repeat("─", inner)), "", "Configure defaults and overrides with cxz edit.")
	if p.loaded && p.info.ConfiguredImage != p.info.Image {
		lines = append(lines, "Saved image: "+p.info.ConfiguredImage)
	}
	if p.busy {
		lines = append(lines, "Working… You can close this page; the request continues.")
	} else if p.loading {
		lines = append(lines, "Refreshing…")
	}
	if p.info.UsageError != "" {
		lines = append(lines, "Cache usage: "+p.info.UsageError)
	}
	if p.statusError != "" {
		lines = append(lines, "", p.statusError)
	}
	if p.message != "" {
		lines = append(lines, "", p.message)
	}
	// Keep the button rows stable; wrap detail text so complete errors are readable.
	var wrapped []string
	for i, line := range lines {
		if i < settingsActionRow+len(settingsActions)+1 {
			wrapped = append(wrapped, clip(line, inner))
		} else {
			wrapped = append(wrapped, strings.Split(ansi.Hardwrap(safeText(line), inner, true), "\n")...)
		}
	}
	height := max(1, m.height-1)
	p.offset = max(0, min(p.offset, max(0, len(wrapped)-height)))
	rows := make([]string, height)
	for i := range rows {
		if p.offset+i < len(wrapped) {
			rows[i] = "  " + wrapped[p.offset+i]
		}
	}
	footer := "↑/↓ select · Enter act · PgUp/PgDn scroll · r refresh · Esc back"
	rows = append(rows, "  "+muted.Render(clip(footer, inner)))
	return screen(strings.Join(rows, "\n"), width, m.height)
}
