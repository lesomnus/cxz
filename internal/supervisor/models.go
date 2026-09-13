package supervisor

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

func (s *Supervisor) publishModels() {
	source := s.modelSource
	if source == "" {
		source = map[bool]string{true: "model/list", false: "initialize"}[s.codex != nil]
	}
	s.event("models", "catalog", "", map[string]any{"models": s.modelOptions, "model": s.session.Model, "effort": s.effort, "source": source}, nil)
}

func isSettingCommand(text string) bool {
	f := strings.Fields(text)
	return len(f) > 0 && (f[0] == "/model" || f[0] == "/effort")
}

func (s *Supervisor) readModels() {
	if s.modelDisabled || (!s.modelRequested.IsZero() && time.Since(s.modelRequested) < time.Minute) {
		return
	}
	s.modelRequested = time.Now()
	s.modelPages = nil
	if s.codex == nil {
		if err := s.write(map[string]any{"type": "control_request", "request_id": "cxz-models", "request": map[string]any{"subtype": "list_models"}}); err != nil {
			s.event("models_status", "unavailable", "", nil, nil)
		}
		return
	}
	if err := s.write(rpc("cxz-models", "model/list", map[string]any{"limit": 100, "includeHidden": false})); err != nil {
		s.modelRequested = time.Time{}
		s.event("models_status", "unavailable", "", nil, nil)
	}
}

func (s *Supervisor) restoreSetting(name string, raw []byte) {
	var v struct{ Value string }
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	if name == "model" {
		s.session.Model = v.Value
	}
	if name == "effort" {
		s.effort = v.Value
	}
}

func (s *Supervisor) configure(c core.Command) (core.Receipt, error) {
	r := core.Receipt{ClientID: c.ClientID}
	f := strings.Fields(c.Text)
	if s.snap.State != "idle" || s.settingPending != "" {
		return r, fmt.Errorf("settings require an idle session with no pending update")
	}
	if len(f) == 1 {
		s.readModels()
		s.publishModels()
		rec := record{Op: "send", Command: c, Status: "accepted"}
		s.receipts[c.ClientID] = rec
		s.event("receipt", "configure", c.ClientID, rec, nil)
		return core.Receipt{ClientID: c.ClientID, Status: "accepted"}, nil
	}
	if len(f) != 2 {
		return r, fmt.Errorf("usage: %s <value>", f[0])
	}
	name, value := strings.TrimPrefix(f[0], "/"), f[1]
	if name == "model" && s.effort != "" {
		return r, fmt.Errorf("reset /effort default before changing models")
	}
	if value == "default" {
		value = ""
	}
	if s.codex != nil && value == "" {
		valid := false
		for _, option := range s.modelOptions {
			if name == "model" && option.Default {
				valid = true
			}
			if name == "effort" && (option.ID == s.session.Model || s.session.Model == "" && option.Default) && option.DefaultEffort != "" {
				valid = true
			}
		}
		if !valid {
			return r, fmt.Errorf("provider has not reported a default; select an explicit catalog value")
		}
	}
	if value != "" {
		valid := false
		for _, m := range s.modelOptions {
			if name == "model" && m.ID == value {
				valid = true
			}
			if name == "effort" && (m.ID == s.session.Model || s.session.Model == "" && m.Default) && slices.Contains(m.Efforts, value) {
				valid = true
			}
		}
		if !valid {
			return r, fmt.Errorf("value not reported for this model by provider; use /model, select a model before /effort")
		}
		// Claude's flag settings layer cannot represent session-only max effort.
		if s.codex == nil && name == "effort" && value == "max" {
			return r, fmt.Errorf("this Claude control transport cannot apply session-only max effort")
		}
	}
	rec := record{Op: "send", Command: c, Status: "delivery_unknown"}
	s.receipts[c.ClientID] = rec
	s.event("intent", "configure", c.ClientID, rec, nil)
	if s.codex != nil {
		s.finishSetting(c.ClientID, true)
		return core.Receipt{ClientID: c.ClientID, Status: "accepted"}, nil
	}
	var model any = value
	if value == "" {
		model = nil
	}
	request := map[string]any{"subtype": "set_model", "model": model}
	if name == "effort" {
		var effort any = value
		if value == "" {
			effort = nil
		}
		request = map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": effort}}
	}
	s.settingPending = c.ClientID
	if err := s.write(map[string]any{"type": "control_request", "request_id": "cxz-setting-" + c.ClientID, "request": request}); err != nil {
		s.finishSetting(c.ClientID, false)
		return r, err
	}
	s.event("setting_status", "pending", c.ClientID, nil, nil)
	return core.Receipt{ClientID: c.ClientID, Status: "pending"}, nil
}

func (s *Supervisor) finishSetting(id string, success bool) {
	r, ok := s.receipts[id]
	if !ok || r.Status != "delivery_unknown" {
		return
	}
	f := strings.Fields(r.Command.Text)
	if len(f) != 2 {
		return
	}
	if s.settingPending == id {
		s.settingPending = ""
	}
	if success {
		value := f[1]
		if value == "default" {
			value = ""
		}
		name := strings.TrimPrefix(f[0], "/")
		payload, _ := json.Marshal(map[string]string{"value": value})
		s.restoreSetting(name, payload)
		s.event("setting", name, id, json.RawMessage(payload), nil)
		s.publishModels()
		r.Status = "accepted"
	} else {
		s.event("setting_status", "rejected; provider did not confirm update", id, nil, nil)
		r.Status = "rejected"
	}
	s.receipts[id] = r
	s.event("receipt", "configure", id, r, nil)
}

func (s *Supervisor) expireSetting() {
	// Do not silently release an unconfirmed setting and send a turn with an
	// unknown configuration. The user can stop/resume to recover from a hung CLI.
	if s.settingPending != "" {
		s.event("setting_status", "awaiting provider; stop/resume if this persists", s.settingPending, nil, nil)
	}
}
