package supervisor

import (
	"bytes"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"path/filepath"
	"testing"
)

type sink struct{ bytes.Buffer }

func (s *sink) Close() error { return nil }
func TestCodexProtocol(t *testing.T) {
	log, e := journal.Open(filepath.Join(t.TempDir(), "events"))
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	input := &sink{}
	s := &Supervisor{session: core.Session{ID: "s", Kind: "codex", Workspace: "/workspaces/test"}, log: log, snap: core.Snapshot{RunID: "run", State: "starting"}, pending: map[string]core.Event{}, receipts: map[string]record{}, stdin: input}
	s.codex = &codexProtocol{s: s}
	s.consume([]byte(`{"id":"cxz-initialize","result":{}}`))
	if !bytes.Contains(input.Bytes(), []byte(`"method":"thread/start"`)) {
		t.Fatal("no thread startup")
	}
	s.consume([]byte(`{"id":"cxz-thread","result":{"thread":{"id":"thread-1"}}}`))
	if s.snap.State != "idle" || s.snap.VendorID != "thread-1" {
		t.Fatal("not ready")
	}
	r, e := s.execute("send", core.Command{RunID: "run", ClientID: "send", Text: "hello"})
	if e != nil || r.Status != "accepted" {
		t.Fatal(r, e)
	}
	s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"turn-1"}}}`))
	s.consume([]byte(`{"id":42,"method":"item/commandExecution/requestApproval","params":{"command":"echo test"}}`))
	if s.snap.State != "waiting_input" {
		t.Fatal("approval did not block")
	}
	_, e = s.execute("reply", core.Command{RunID: "run", ClientID: "reply", RequestID: "42", Allow: true})
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(input.Bytes(), []byte(`"id":42,"result":{"decision":"accept"}`)) {
		t.Fatal("response did not preserve numeric correlation id")
	}
	s.consume([]byte(`{"id":"question","method":"item/tool/requestUserInput","params":{"questions":[{"id":"color","question":"Choose a color"}]}}`))
	if _, e = s.execute("reply", core.Command{RunID: "run", ClientID: "bad", RequestID: `"question"`, Allow: true}); e == nil {
		t.Fatal("missing answer allowed")
	}
	if _, e = s.execute("reply", core.Command{RunID: "run", ClientID: "answer", RequestID: `"question"`, Allow: true, Answers: map[string]string{"color": "Blue"}}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.execute("interrupt", core.Command{RunID: "run", ClientID: "interrupt"}); e != nil {
		t.Fatal(e)
	}
	s.consume([]byte(`{"method":"turn/completed","params":{"turn":{"id":"turn-1","status":"interrupted"}}}`))
	if s.snap.State != "idle" {
		t.Fatal("not idle")
	}
	events := log.All()
	found := false
	for _, v := range events {
		if v.Kind == "turn_end" && v.Text == "interrupted" {
			found = true
		}
	}
	if !found {
		b, _ := json.Marshal(events)
		t.Fatal(string(b))
	}
}
