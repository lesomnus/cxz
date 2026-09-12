package supervisor

import (
	"bytes"
	"encoding/json"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
	"path/filepath"
	"testing"
)

type sink struct{ bytes.Buffer }

func TestCodexExternalAuthentication(t *testing.T) {
	log, err := journal.Open(filepath.Join(t.TempDir(), "events"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	input := &sink{}
	s := &Supervisor{session: core.Session{Kind: "codex", Workspace: "/workspace"}, log: log, stdin: input, snap: core.Snapshot{State: "starting"}, pending: map[string]core.Event{}}
	requests := 0
	c := &codexProtocol{s: s, token: func(previous string, refresh bool) (accounts.Token, error) {
		requests++
		if refresh && previous != "subject-work" {
			t.Fatal("lost account pin")
		}
		return accounts.Token{AccessToken: "synthetic-secret", AccountID: "subject-work"}, nil
	}}
	c.consume([]byte(`{"id":"cxz-initialize","result":{}}`))
	if bytes.Contains(input.Bytes(), []byte("thread/start")) || !bytes.Contains(input.Bytes(), []byte("chatgptAuthTokens")) {
		t.Fatal("thread started before auth")
	}
	c.consume([]byte(`{"id":"cxz-auth","result":{"type":"chatgptAuthTokens"}}`))
	if !bytes.Contains(input.Bytes(), []byte("thread/start")) {
		t.Fatal("no thread after auth")
	}
	c.consume([]byte(`{"id":42,"method":"account/chatgptAuthTokens/refresh","params":{"previousAccountId":"subject-work","reason":"unauthorized"}}`))
	if requests != 2 || len(s.pending) != 0 {
		t.Fatal("refresh incorrectly routed as approval")
	}
	for _, event := range log.All() {
		raw, _ := json.Marshal(event)
		if bytes.Contains(raw, []byte("synthetic-secret")) {
			t.Fatal("token journaled")
		}
	}
	if !bytes.Contains(input.Bytes(), []byte(`"id":42`)) {
		t.Fatal("refresh correlation lost")
	}
}

func (s *sink) Close() error { return nil }
func TestCodexProtocol(t *testing.T) {
	log, e := journal.Open(filepath.Join(t.TempDir(), "events"))
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	input := &sink{}
	s := &Supervisor{session: core.Session{ID: "s", Kind: "codex", Model: "test-model", Workspace: "/workspaces/test"}, log: log, snap: core.Snapshot{RunID: "run", State: "starting"}, pending: map[string]core.Event{}, receipts: map[string]record{}, stdin: input}
	s.codex = &codexProtocol{s: s}
	s.consume([]byte(`{"id":"cxz-initialize","result":{}}`))
	if !bytes.Contains(input.Bytes(), []byte(`"method":"thread/start"`)) {
		t.Fatal("no thread startup")
	}
	if !bytes.Contains(input.Bytes(), []byte(`"model":"test-model"`)) {
		t.Fatal("model not passed")
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
	if _, e = s.execute("reply", core.Command{RunID: "previous-run", ClientID: "stale-run", RequestID: "42", Allow: true}); e == nil {
		t.Fatal("stale run accepted")
	}
	if _, e = s.execute("reply", core.Command{RunID: "run", ClientID: "stale-request", RequestID: "42", Allow: true}); e == nil {
		t.Fatal("resolved approval accepted again")
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
