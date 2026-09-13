package agentview

import "testing"

func TestNativeToolSummaries(t *testing.T) {
	a, ok := ToolView("claude", "Write", []byte(`{"file_path":"/tmp/main.go","content":"one\ntwo\n"}`))
	if !ok || a.Files[0].Lines != 2 || a.Files[0].Measure != "content" {
		t.Fatalf("%+v", a)
	}
	a, ok = ToolView("claude", "Edit", []byte(`{"file_path":"/tmp/main.go","old_string":"one\ntwo","new_string":"three","replace_all":true}`))
	if !ok || a.Files[0].Removed != 2 || a.Files[0].Added != 1 || !a.Files[0].PerMatch {
		t.Fatalf("%+v", a)
	}
	a, ok = ToolView("codex", "", []byte(`{"item":{"type":"fileChange","changes":[{"path":"x.go","kind":{"type":"update","move_path":"y.go"},"diff":"--- a/x.go\n+++ b/y.go\n@@ -1,2 +1,2 @@\n same\n-old\n+new\n"}]}}`))
	if !ok || a.Files[0].Added != 1 || a.Files[0].Removed != 1 || a.Files[0].MovePath != "y.go" {
		t.Fatalf("%+v", a)
	}
	a, _ = ToolView("codex", "", []byte(`{"item":{"type":"fileChange","changes":[{"path":"x.go","kind":{"type":"delete"}}]}}`))
	if a.Files[0].Measure != "unknown" {
		t.Fatal("invented change count")
	}
}
