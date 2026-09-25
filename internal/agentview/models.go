package agentview

import "encoding/json"

// ModelOption is a provider-reported capability, never a hard-coded model list.
type ModelOption struct {
	ID            string   `json:"id"`
	ResolvedID    string   `json:"resolved_id,omitempty"`
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
			ID, Model, Value, DisplayName, ResolvedModel string
			Hidden, Disabled, IsDefault                  bool
			SupportedEffortLevels                        []string
			SupportedReasoningEfforts                    []struct{ ReasoningEffort string }
			DefaultReasoningEffort                       string
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
		out = append(out, ModelOption{ID: id, ResolvedID: v.ResolvedModel, Name: v.DisplayName, Efforts: efforts, DefaultEffort: v.DefaultReasoningEffort, Default: v.IsDefault || provider == "claude" && id == "default"})
	}
	return out
}

// SelectedModel resolves the provider's applied ID and catalog aliases in one
// place for both the runtime's validation and the client's effort selector.
func SelectedModel(options []ModelOption, selected, applied string) (ModelOption, bool) {
	for _, option := range options {
		if option.ID == selected && (applied == "" || option.ID == applied || option.ResolvedID == applied) {
			return option, true
		}
	}
	target := applied
	if target == "" {
		target = selected
	}
	if target != "" && target != "default" {
		for _, option := range options {
			if option.ID == target || option.ResolvedID == target {
				return option, true
			}
		}
		// Older catalogs can advertise an alias without its resolved ID. An
		// explicitly selected alias still has its own reported capabilities.
		for _, option := range options {
			if option.ID == selected && option.ResolvedID == "" {
				return option, true
			}
		}
		return ModelOption{}, false
	}
	for _, option := range options {
		// ID also supports catalogs journaled before Default was normalized.
		if option.Default || option.ID == "default" {
			return option, true
		}
	}
	return ModelOption{}, false
}
