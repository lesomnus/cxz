package supervisor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Keep the confirmed preference in the existing journal and the applied value
// in the run's catalog. A default preference is not itself a reasoning level.
func (s *Supervisor) readClaudeSettings(setting string) {
	if s.codex != nil || setting == "" && s.settingPending != "" {
		return
	}
	s.settingsSequence++
	s.settingsRequest = fmt.Sprintf("cxz-settings-%d", s.settingsSequence)
	s.settingsFor = setting
	if err := s.write(map[string]any{"type": "control_request", "request_id": s.settingsRequest, "request": map[string]any{"subtype": "get_settings"}}); err != nil {
		s.receiveClaudeSettings(s.settingsRequest, false, nil)
	}
}

func (s *Supervisor) receiveClaudeSettings(id string, success bool, raw []byte) {
	if id != s.settingsRequest || id == "" {
		return
	}
	setting := s.settingsFor
	s.settingsRequest, s.settingsFor = "", ""
	// A background read that preceded a change cannot confirm the new value.
	if setting == "" && s.settingPending != "" {
		return
	}
	var state struct {
		Applied *struct{ Model, Effort string } `json:"applied"`
	}
	valid := success && json.Unmarshal(raw, &state) == nil && state.Applied != nil && state.Applied.Model != ""
	if valid {
		s.appliedModel, s.appliedEffort = state.Applied.Model, state.Applied.Effort
	} else {
		s.appliedModel, s.appliedEffort = "", ""
	}
	if setting != "" {
		fields := strings.Fields(s.receipts[setting].Command.Text)
		confirmed := valid && len(fields) == 2 && (fields[1] == "default" || fields[1] == s.appliedEffort)
		s.finishSetting(setting, confirmed)
		if confirmed {
			return // finishSetting publishes the confirmed preference and state.
		}
	}
	s.publishModels()
}
