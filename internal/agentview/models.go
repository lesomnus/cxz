package agentview

import "encoding/json"

// ModelOption is a provider-reported capability, never a hard-coded model list.
type ModelOption struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Efforts       []string `json:"efforts,omitempty"`
	DefaultEffort string   `json:"default_effort,omitempty"`
	Default       bool     `json:"default,omitempty"`
}

func Models(provider string, raw []byte) []ModelOption {
	var root struct {
		Models []json.RawMessage `json:"models"`
		Data   []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	rows := root.Models
	if provider == "codex" {
		rows = root.Data
	}
	var out []ModelOption
	for _, raw := range rows {
		var v struct {
			ID, Model, Value, DisplayName string
			Hidden, Disabled, IsDefault   bool
			SupportedEffortLevels         []string
			SupportedReasoningEfforts     []struct{ ReasoningEffort string }
			DefaultReasoningEffort        string
		}
		if json.Unmarshal(raw, &v) != nil || v.Hidden || v.Disabled {
			continue
		}
		id := v.Value
		if provider == "codex" {
			id = v.Model
			if id == "" {
				id = v.ID
			}
		}
		if id == "" {
			continue
		}
		efforts := v.SupportedEffortLevels
		for _, e := range v.SupportedReasoningEfforts {
			efforts = append(efforts, e.ReasoningEffort)
		}
		out = append(out, ModelOption{ID: id, Name: v.DisplayName, Efforts: efforts, DefaultEffort: v.DefaultReasoningEffort, Default: v.IsDefault})
	}
	return out
}
