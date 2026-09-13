package tui

import (
	"encoding/json"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/cxz/internal/agentview"
)

func (m *model) modelCommand(text string) tea.Cmd {
	s := m.current()
	if s == nil {
		return nil
	}
	if len(strings.Fields(text)) > 1 {
		return m.action("send", text)
	}
	m.recordLocal("/model", m.modelReport())
	if s.State == "idle" {
		return m.action("send", text)
	}
	return nil
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
	lines = append(lines, "", "/model <id> · /effort <level> · /effort default", "Claude: initialization catalog; restart to refresh. Codex: catalog refreshes every idle minute.", "Changes require idle. Provider acknowledgement is shown in the transcript; no chat prompt is sent.")
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
