package agentview

import "testing"

func TestBackgroundCommandAndDescription(t *testing.T) {
	for _, tc := range []struct {
		name, event, command, description string
	}{
		{"Claude description", `{"type":"system","subtype":"task_started","task_id":"bg","is_backgrounded":true,"description":"Check timeout handling in bash"}`, "", "Check timeout handling in bash"},
		{"Codex command", `{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"bg","command":"sleep 60"}]}`, "sleep 60", ""},
		{"legacy Codex command", `{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"bg","description":"sleep 60"}]}`, "sleep 60", ""},
		{"both fields", `{"type":"system","subtype":"task_started","task_id":"bg","is_backgrounded":true,"command":"sleep 60","description":"Wait for server startup"}`, "sleep 60", "Wait for server startup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var state BackgroundState
			state.ApplyAt([]byte(tc.event), 1, 1000)
			state.ApplyAt([]byte(tc.event), 2, 2000)
			task := state.Tasks["bg"]
			if task.Command != tc.command || task.Description != tc.description || task.UpdatedSeq != 1 || task.StartedAtMS != 1000 {
				t.Fatal(task)
			}
		})
	}
}

func TestBackgroundCommandUpgradeClearsLegacyDescription(t *testing.T) {
	state := BackgroundState{Tasks: map[string]BackgroundTask{
		"bg":       {ID: "bg", Description: "old command", Active: true, StartedAtMS: 1000},
		"finished": {ID: "finished", Description: "finished command", Status: "inactive"},
	}}
	state.ApplyAt([]byte(`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"bg","command":"new command"}]}`), 2, 2000)
	if task := state.Tasks["bg"]; task.Command != "new command" || task.Description != "" || task.StartedAtMS != 1000 {
		t.Fatal(task)
	}
	if task := state.Tasks["finished"]; task.Command != "finished command" || task.Description != "" || task.Active {
		t.Fatal(task)
	}
}

func TestBackgroundCommandPatchPreservesDescription(t *testing.T) {
	var state BackgroundState
	state.Apply([]byte(`{"type":"system","subtype":"task_started","task_id":"bg","is_backgrounded":true,"command":"sleep 60","description":"Wait for server startup"}`))
	state.Apply([]byte(`{"type":"system","subtype":"task_updated","task_id":"bg","patch":{"command":"sleep 30"}}`))
	if task := state.Tasks["bg"]; task.Command != "sleep 30" || task.Description != "Wait for server startup" {
		t.Fatal(task)
	}
}

func TestBackgroundLifecycle(t *testing.T) {
	var s BackgroundState
	s.Apply([]byte(`{"type":"system","subtype":"task_started","task_id":"foreground","is_backgrounded":false}`))
	s.Apply([]byte(`{"type":"system","subtype":"task_notification","task_id":"foreground","status":"completed"}`))
	if len(s.Tasks) != 0 {
		t.Fatal("foreground task tracked")
	}
	s.Apply([]byte(`{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"bg","description":"Sleep"}]}`))
	s.Apply([]byte(`{"type":"system","subtype":"task_started","task_id":"bg","tool_use_id":"tool","is_backgrounded":true}`))
	if v := s.Tasks["bg"]; !v.Active || v.ToolID != "tool" || v.Description != "Sleep" {
		t.Fatal(v)
	}
	s.Apply([]byte(`{"type":"system","subtype":"background_tasks_changed","tasks":[]}`))
	if v := s.Tasks["bg"]; v.Active || v.Status == "completed" {
		t.Fatal(v)
	}
	s.Apply([]byte(`{"type":"system","subtype":"task_updated","task_id":"bg","patch":{"status":"completed"}}`))
	s.Apply([]byte(`{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed","summary":"Done","output_file":"/not/read"}`))
	if v := s.Tasks["bg"]; v.Active || v.Status != "completed" || v.OutputFile != "/not/read" || v.ToolID != "tool" {
		t.Fatal(v)
	}
}

func TestBackgroundOrderingChangesOnlyWithTaskState(t *testing.T) {
	var state BackgroundState
	active := []byte(`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[{"task_id":"bg","description":"Build"}]}`)
	empty := []byte(`{"type":"background","provider":"codex","subtype":"background_tasks_changed","tasks":[]}`)
	state.ApplyAt(active, 10, 1000)
	state.ApplyAt(active, 20, 2000)
	if task := state.Tasks["bg"]; task.UpdatedSeq != 10 || task.UpdatedAtMS != 1000 || !task.Active {
		t.Fatal(task)
	}
	state.ApplyAt(empty, 30, 3000)
	state.ApplyAt(empty, 40, 4000)
	if task := state.Tasks["bg"]; task.UpdatedSeq != 30 || task.UpdatedAtMS != 3000 || task.Active || task.Status != "inactive" {
		t.Fatal(task)
	}
	state.ApplyAt(active, 50, 5000)
	if task := state.Tasks["bg"]; task.UpdatedSeq != 50 || !task.Active {
		t.Fatal(task)
	}
}

func TestBackgroundCompletionMetadataUsesEventSequence(t *testing.T) {
	var state BackgroundState
	state.ApplyAt([]byte(`{"type":"system","subtype":"task_started","task_id":"bg","is_backgrounded":true}`), 1, 1000)
	state.ApplyAt([]byte(`{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed","summary":"Done"}`), 2, 2000)
	state.ApplyAt([]byte(`{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed","summary":"Done"}`), 3, 3000)
	if task := state.Tasks["bg"]; task.UpdatedSeq != 2 || task.UpdatedAtMS != 2000 || task.Active {
		t.Fatal(task)
	}
}

func TestBackgroundDurationUsesFirstObservationAndStopsWhenInactive(t *testing.T) {
	var state BackgroundState
	started := []byte(`{"type":"system","subtype":"task_started","task_id":"bg","is_backgrounded":true}`)
	state.ApplyAt(started, 1, 1000)
	state.ApplyAt([]byte(`{"type":"system","subtype":"task_updated","task_id":"bg","patch":{"description":"still building"}}`), 2, 2000)
	if task := state.Tasks["bg"]; task.StartedAtMS != 1000 || task.FinishedAtMS != 0 {
		t.Fatal(task)
	}
	state.ApplyAt([]byte(`{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed"}`), 3, 5000)
	state.ApplyAt([]byte(`{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed","summary":"later summary"}`), 4, 6000)
	if task := state.Tasks["bg"]; task.StartedAtMS != 1000 || task.FinishedAtMS != 5000 {
		t.Fatal(task)
	}
	state.ApplyAt(started, 5, 7000)
	if task := state.Tasks["bg"]; task.StartedAtMS != 1000 || task.FinishedAtMS != 0 {
		t.Fatal(task)
	}
}

func TestLegacyActiveTaskUpdateDoesNotInventAStartTime(t *testing.T) {
	state := BackgroundState{Tasks: map[string]BackgroundTask{"bg": {ID: "bg", Active: true, Status: "working"}}}
	state.ApplyAt([]byte(`{"type":"system","subtype":"task_updated","task_id":"bg","patch":{"description":"new detail"}}`), 2, 5000)
	if task := state.Tasks["bg"]; task.StartedAtMS != 0 || task.FinishedAtMS != 0 {
		t.Fatal(task)
	}
}
