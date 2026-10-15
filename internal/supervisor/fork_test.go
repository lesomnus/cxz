package supervisor

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func TestCodexForkUsesPrivateSnapshotAndResumesTheChildAfterward(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.modelDisabled = true
	s.session.Fork = &core.ForkOrigin{SessionID: "parent", VendorID: "parent-thread", NativePath: "/private/child/sessions/parent.jsonl"}
	s.snap.VendorID = ""
	s.codex.startThread()
	var request struct {
		Method string
		Params map[string]any
	}
	if err := json.Unmarshal(input.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.Method != "thread/fork" || request.Params["threadId"] != "parent-thread" || request.Params["path"] != s.session.Fork.NativePath || request.Params["excludeTurns"] != true {
		t.Fatal("not an independent native fork", request)
	}
	s.consume([]byte(`{"id":"cxz-thread","result":{"thread":{"id":"child-thread"}}}`))
	if s.snap.VendorID != "child-thread" || s.snap.State != "idle" {
		t.Fatal("child did not become ready", s.snap)
	}
	input.Reset()
	s.codex.startThread()
	if err := json.Unmarshal(input.Bytes(), &request); err != nil {
		t.Fatal(err)
	}
	if request.Method != "thread/resume" || request.Params["threadId"] != "child-thread" {
		t.Fatal("restarting forked the parent again", request)
	}
}

func TestClaudeForkUsesNativeForkOnlyUntilTheChildHasAnIdentity(t *testing.T) {
	session := core.Session{Kind: "claude", Model: "chosen-model", Fork: &core.ForkOrigin{VendorID: "parent-thread"}}
	args := claudeSessionArgs(session, "high", "")
	if !slices.Contains(args, "--fork-session") || !slices.Contains(args, "parent-thread") || !slices.Contains(args, "chosen-model") || !slices.Contains(args, "high") {
		t.Fatal("fork settings missing", args)
	}
	restarted := claudeSessionArgs(session, "high", "child-thread")
	if slices.Contains(restarted, "--fork-session") || slices.Contains(restarted, "parent-thread") || !slices.Contains(restarted, "child-thread") {
		t.Fatal("restart did not resume child", restarted)
	}
}

func TestForkControlBoundaryDropsInheritedReceiptsButRestoresPreferences(t *testing.T) {
	events := []core.Event{
		{Seq: 1, Kind: "state", Text: "idle", RunID: "parent-run"},
		{Seq: 2, Kind: "vendor", Text: "parent-thread"},
		{Seq: 3, Kind: "receipt", Payload: json.RawMessage(`{"op":"send","command":{"client_id":"parent-send"}}`)},
		{Seq: 4, Kind: core.ForkBoundaryKind},
		{Seq: 5, Kind: "setting", Text: "effort", Payload: json.RawMessage(`{"value":"high"}`)},
		{Seq: 6, Kind: "permission", Text: "ask"},
	}
	restored := resumeEvents(events)
	if !reflect.DeepEqual(restored, events[4:]) {
		t.Fatal("parent control state survived", restored)
	}
	snapshot := Replay(events)
	if snapshot.VendorID != "" || snapshot.RunID != "" || snapshot.PermissionMode != "ask" {
		t.Fatal("parent identity survived", snapshot)
	}
	for split := 0; split < len(events); split++ {
		if got := ReplayFrom(Replay(events[:split]), events[split:]); !reflect.DeepEqual(got, snapshot) {
			t.Fatal("incremental boundary mismatch", split, got, snapshot)
		}
	}
}
