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
