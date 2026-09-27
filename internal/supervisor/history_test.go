package supervisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/lesomnus/cxz/internal/journal"
)

func TestHistoryRetentionPreservesResumeAndIdempotence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "events.jsonl")
	log, err := journal.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := &Supervisor{session: core.Session{ID: "session", Kind: "codex"}, log: log, snap: core.Snapshot{RunID: "run", State: "idle"}, receipts: map[string]record{}}
	s.event("vendor", "provider-thread", "", nil, nil)
	s.event("permission", "ask", "", nil, nil)
	s.event("setting", "effort", "", map[string]string{"value": "high"}, nil)
	original := core.Command{RunID: "run", ClientID: "old", Text: "old private input"}
	r := record{Op: "send", Command: original, Status: "accepted"}
	s.receipts["old"] = r
	s.event("receipt", "send", "old", r, nil)
	for i := range 8 {
		s.event("input", "prompt "+strings.Repeat("x", 10), "", nil, nil)
		s.event("state", "working", "", nil, nil)
		s.event("assistant", strings.Repeat("output", 40000), "", nil, nil)
		s.event("turn_end", "completed", "", nil, nil)
		s.event("state", "idle", "", nil, nil)
		_ = i
	}
	before := s.snap.LastSeq
	if err = historypolicy.Save(root, historypolicy.Policy{MaxMiB: 1}); err != nil {
		t.Fatal(err)
	}
	authorized := false
	if err = s.retainHistory(root, func() error { authorized = true; return nil }); err != nil || !authorized {
		t.Fatal(err, authorized)
	}
	events := log.All()
	if events[0].Kind != core.HistoryCheckpointKind || events[len(events)-1].Seq != before+1 {
		t.Fatal("missing checkpoint or sequence reset")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), original.Text) {
		t.Fatal("old command body retained")
	}
	if len(data) > historypolicy.MiB {
		t.Fatal("retained display history exceeds budget", len(data))
	}
	if snap := Replay(events); snap.VendorID != "provider-thread" || snap.PermissionMode != "ask" || snap.State != "idle" {
		t.Fatal(snap)
	}
	log.Close()
	log, err = journal.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	restored := &Supervisor{log: log, receipts: map[string]record{}, snap: core.Snapshot{RunID: "new-run"}}
	for _, e := range resumeEvents(log.All()) {
		if e.Kind == "setting" {
			restored.restoreSetting(e.Text, e.Payload)
		}
		if e.Kind == "receipt" {
			var rec record
			if err = json.Unmarshal(e.Payload, &rec); err != nil {
				t.Fatal(err)
			}
			restored.receipts[rec.Command.ClientID] = rec
		}
	}
	if restored.effort != "high" {
		t.Fatal("effort lost")
	}
	receipt, err := restored.execute("send", original)
	if err != nil || receipt.Status != "accepted" {
		t.Fatal("duplicate re-delivered", receipt, err)
	}
	original.Text = "different"
	if _, err = restored.execute("send", original); err == nil {
		t.Fatal("changed duplicate accepted")
	}
}

func TestHistoryRetentionDefersActiveWorkAndDisabledPolicy(t *testing.T) {
	root := t.TempDir()
	l, err := journal.Open(filepath.Join(root, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := &Supervisor{log: l, snap: core.Snapshot{State: "working"}, receipts: map[string]record{}}
	s.event("assistant", strings.Repeat("x", 2*historypolicy.MiB), "", nil, nil)
	historypolicy.Save(root, historypolicy.Policy{MaxMiB: 1})
	authorize := func() error { t.Fatal("active work attempted pruning"); return nil }
	if err = s.retainHistory(root, authorize); err != nil {
		t.Fatal(err)
	}
	s.snap.State = "idle"
	historypolicy.Save(root, historypolicy.Policy{Disabled: true})
	if err = s.retainHistory(root, authorize); err != nil {
		t.Fatal(err)
	}
}
