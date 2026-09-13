package agentview

import "encoding/json"

// BackgroundTask describes provider-reported work, not a foreground tool call.
type BackgroundTask struct {
	ID          string `json:"task_id"`
	ToolID      string `json:"tool_use_id"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	OutputFile  string `json:"output_file"`
	Active      bool   `json:"-"`
}

type backgroundEvent struct {
	BackgroundTask
	Type         string           `json:"type"`
	Subtype      string           `json:"subtype"`
	Backgrounded bool             `json:"is_backgrounded"`
	Tasks        []BackgroundTask `json:"tasks"`
	Patch        BackgroundTask   `json:"patch"`
}

// IsBackgroundEvent recognizes Claude's native system envelope. Raw journal
// records remain intact; the API may expose these as background telemetry.
func IsBackgroundEvent(raw []byte) bool {
	var e backgroundEvent
	if json.Unmarshal(raw, &e) != nil || e.Type != "system" {
		return false
	}
	switch e.Subtype {
	case "background_tasks_changed", "task_started", "task_updated", "task_notification":
		return true
	}
	return false
}

type BackgroundState struct {
	Tasks    map[string]BackgroundTask
	Snapshot bool
}

func (s *BackgroundState) Apply(raw []byte) {
	var e backgroundEvent
	if !IsBackgroundEvent(raw) || json.Unmarshal(raw, &e) != nil {
		return
	}
	if s.Tasks == nil {
		s.Tasks = map[string]BackgroundTask{}
	}
	if e.Subtype == "background_tasks_changed" {
		s.Snapshot = true
		for id, t := range s.Tasks {
			t.Active = false
			s.Tasks[id] = t
		}
		for _, t := range e.Tasks {
			if t.ID == "" {
				continue
			}
			old := s.Tasks[t.ID]
			mergeTask(&old, t)
			old.Active = true
			old.Status = "working"
			s.Tasks[t.ID] = old
		}
		return
	}
	if e.ID == "" {
		return
	}
	t, known := s.Tasks[e.ID]
	if !known && !(e.Subtype == "task_started" && e.Backgrounded) {
		return
	}
	mergeTask(&t, e.BackgroundTask)
	if e.Subtype == "task_started" && e.Backgrounded {
		t.Active = true
		t.Status = "working"
	}
	if e.Subtype == "task_updated" {
		mergeTask(&t, e.Patch)
	}
	switch t.Status {
	case "completed", "failed", "stopped", "cancelled", "canceled":
		t.Active = false
	}
	s.Tasks[e.ID] = t
}

func mergeTask(dst *BackgroundTask, src BackgroundTask) {
	if src.ID != "" {
		dst.ID = src.ID
	}
	if src.ToolID != "" {
		dst.ToolID = src.ToolID
	}
	if src.Description != "" {
		dst.Description = src.Description
	}
	if src.Status != "" {
		dst.Status = src.Status
	}
	if src.Summary != "" {
		dst.Summary = src.Summary
	}
	if src.OutputFile != "" {
		dst.OutputFile = src.OutputFile
	}
}
