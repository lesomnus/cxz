package agentview

import "testing"

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
