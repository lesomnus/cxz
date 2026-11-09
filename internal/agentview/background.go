package agentview

import (
	"encoding/json"
	"maps"
)

// BackgroundTask describes provider-reported work, not a foreground tool call.
type BackgroundTask struct {
	ID          string `json:"task_id"`
	ToolID      string `json:"tool_use_id"`
	Command     string `json:"command,omitempty"` // Executable text, separate from the provider's description.
	Description string `json:"description"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	OutputFile  string `json:"output_file"`
	Active      bool   `json:"active"`
	UpdatedSeq  uint64 `json:"updated_seq,omitempty,string"`
	UpdatedAtMS int64  `json:"updated_at_ms,omitempty"`
	// Timing follows observed transitions, not provider-reported process clocks.
	StartedAtMS  int64 `json:"started_at_ms,omitempty"`
	FinishedAtMS int64 `json:"finished_at_ms,omitempty"`
}

type backgroundEvent struct {
	BackgroundTask
	Type         string           `json:"type"`
	Provider     string           `json:"provider"`
	Subtype      string           `json:"subtype"`
	Backgrounded bool             `json:"is_backgrounded"`
	Tasks        []BackgroundTask `json:"tasks"`
	Patch        BackgroundTask   `json:"patch"`
}

// IsBackgroundEvent recognizes native Claude telemetry and normalized provider
// snapshots. Raw journal records remain intact.
func IsBackgroundEvent(raw []byte) bool {
	var e backgroundEvent
	if json.Unmarshal(raw, &e) != nil {
		return false
	}
	if e.Type == "background" && e.Provider == "codex" {
		return e.Subtype == "background_tasks_changed" || e.Subtype == "background_status"
	}
	if e.Type != "system" {
		return false
	}
	switch e.Subtype {
	case "background_tasks_changed", "task_started", "task_updated", "task_notification":
		return true
	}
	return false
}

type BackgroundState struct {
	Tasks       map[string]BackgroundTask
	Snapshot    bool
	Unavailable bool `json:"Unavailable,omitempty"`
}

// ApplyAt records when a task actually changes, rather than every provider poll.
func (s *BackgroundState) ApplyAt(raw []byte, seq uint64, timeMS int64) {
	previous := maps.Clone(s.Tasks)
	s.Apply(raw)
	for id, task := range s.Tasks {
		old, exists := previous[id]
		if exists && old == task {
			continue
		}
		if task.Active {
			if task.StartedAtMS == 0 && (!exists || !old.Active) {
				task.StartedAtMS = timeMS
			}
			task.FinishedAtMS = 0
		} else if old.Active {
			task.FinishedAtMS = timeMS
		}
		task.UpdatedSeq = seq
		task.UpdatedAtMS = timeMS
		s.Tasks[id] = task
	}
}

func (s *BackgroundState) Apply(raw []byte) {
	var e backgroundEvent
	if !IsBackgroundEvent(raw) || json.Unmarshal(raw, &e) != nil {
		return
	}
	if s.Tasks == nil {
		s.Tasks = map[string]BackgroundTask{}
	}
	if e.Subtype == "background_status" {
		s.Unavailable = true
		s.Snapshot = false
		return
	}
	s.Unavailable = false
	if e.Subtype == "background_tasks_changed" {
		s.Snapshot = true
		for id, t := range s.Tasks {
			if e.Provider == "codex" {
				t = codexBackgroundTask(t)
			}
			t.Active = false
			if e.Provider == "codex" {
				t.Status = "inactive"
			}
			s.Tasks[id] = t
		}
		for _, t := range e.Tasks {
			if t.ID == "" {
				continue
			}
			if e.Provider == "codex" {
				t = codexBackgroundTask(t)
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

// Older cxz Codex snapshots stored the terminal command as its description.
// Only that provider's snapshots carry this legacy meaning; Claude descriptions
// must remain natural language, even when they happen to resemble shell code.
func codexBackgroundTask(t BackgroundTask) BackgroundTask {
	if t.Command == "" {
		t.Command, t.Description = t.Description, ""
	}
	return t
}

func mergeTask(dst *BackgroundTask, src BackgroundTask) {
	if src.ID != "" {
		dst.ID = src.ID
	}
	if src.ToolID != "" {
		dst.ToolID = src.ToolID
	}
	if src.Command != "" {
		dst.Command = src.Command
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
