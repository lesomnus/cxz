package tui

import (
	"encoding/json"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/agentview"
)

func (m *model) modelCommand(text string) tea.Cmd {
	return m.openModelPicker(text)
}

func (m *model) modelReport() string {
	s := m.current()
	if s == nil {
		return "Select a session first."
	}
	lines := []string{"Model / effort · " + s.Agent, "No provider catalog received. Restart an updated agent if this persists."}
	for i := len(m.events[s.Id]) - 1; i >= 0; i-- {
		e := m.events[s.Id][i]
		if e.Kind != "models" || e.RunId != "" && e.RunId != s.RunId {
			continue
		}
		var v struct {
			Models                []agentview.ModelOption
			Model, Effort, Source string
		}
		if json.Unmarshal(e.Payload, &v) != nil {
			continue
		}
		if v.Model == "" {
			v.Model = "provider default"
		}
		if v.Effort == "" {
			v.Effort = "provider default"
		}
		lines = []string{"Model / effort · " + s.Agent, "Selected model: " + v.Model + " · effort: " + v.Effort, "Source: " + v.Source + " (provider-reported snapshot)", "Observed: " + time.UnixMilli(e.TimeMs).Local().Format("01-02 15:04:05"), ""}
		for _, option := range v.Models {
			lines = append(lines, option.ID+" · "+option.Name, "  effort: "+strings.Join(option.Efforts, ", "))
		}
		if len(v.Models) == 0 {
			lines = append(lines, "This provider did not report model choices.")
		}
		break
	}
	lines = append(lines, "", "/model <id> · /effort <level> · /effort default", "Catalog refreshes every idle minute (Claude: list_models; Codex: model/list). Older Claude CLIs may only support the initial catalog.", "Changes require idle. Provider acknowledgement is shown in the transcript; no chat prompt is sent.")
	return strings.Join(lines, "\n")
}

func (m *model) selectedModelLabel() string {
	s := m.current()
	if s == nil {
		return ""
	}
	for i := len(m.events[s.Id]) - 1; i >= 0; i-- {
		e := m.events[s.Id][i]
		if e.Kind != "models" || e.RunId != "" && e.RunId != s.RunId {
			continue
		}
		var v struct{ Model, Effort string }
		if json.Unmarshal(e.Payload, &v) != nil {
			continue
		}
		if v.Model == "" {
			v.Model = "default"
		}
		if v.Effort != "" {
			v.Model += " · " + v.Effort
		}
		return pickerLabel(v.Model)
	}
	return pickerLabel(s.Model)
}
