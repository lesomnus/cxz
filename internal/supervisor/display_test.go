package supervisor

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/journal"
)

func displaySupervisor(t *testing.T, provider string) (*Supervisor, *sink) {
	t.Helper()
	l, err := journal.Open(filepath.Join(t.TempDir(), "events"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	input := &sink{}
	s := &Supervisor{session: core.Session{ID: "s", Kind: provider}, log: l, stdin: input, snap: core.Snapshot{State: "idle", RunID: "run", VendorID: "thread"}, pending: map[string]core.Event{}, receipts: map[string]record{}}
	if provider == "codex" {
		s.codex = &codexProtocol{s: s}
	}
	return s, input
}

func TestNativeCompaction(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		s, input := displaySupervisor(t, provider)
		cmd := core.Command{RunID: "run", ClientID: "compact", Text: "/compact"}
		if _, err := s.execute("send", cmd); err != nil {
			t.Fatal(err)
		}
		before := input.Len()
		if _, err := s.execute("send", cmd); err != nil || input.Len() != before {
			t.Fatal("compaction retried", err)
		}
		var wire map[string]json.RawMessage
		if json.Unmarshal(bytes.TrimSpace(input.Bytes()), &wire) != nil {
			t.Fatal(input.String())
		}
		if provider == "codex" {
			if string(wire["method"]) != `"thread/compact/start"` || bytes.Contains(input.Bytes(), []byte(`"turn/start"`)) {
				t.Fatal(input.String())
			}
			s.consume([]byte(`{"method":"turn/started","params":{"turn":{"id":"compact-turn"}}}`))
			s.consume([]byte(`{"method":"item/completed","params":{"item":{"type":"contextCompaction","id":"compact-item"}}}`))
			s.consume([]byte(`{"method":"turn/completed","params":{"turn":{"id":"compact-turn","status":"completed"}}}`))
		} else {
			if !bytes.Contains(input.Bytes(), []byte(`"content":"/compact"`)) {
				t.Fatal(input.String())
			}
			s.consume([]byte(`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"manual","pre_tokens":1200}}`))
			s.consume([]byte(`{"type":"result","subtype":"success","result":"Compacted"}`))
		}
		if s.snap.State != "idle" {
			t.Fatal("compaction never completed")
		}
		found := false
		for _, e := range s.log.All() {
			if e.Kind == "compact" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing durable compact boundary")
		}
		if _, err := s.execute("send", core.Command{RunID: "stale", ClientID: "stale", Text: "/compact"}); err == nil {
			t.Fatal("stale compaction accepted")
		}
	}
}

func TestQuotaFailureNeverFailsTurn(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		s, input := displaySupervisor(t, provider)
		s.snap.State = "working"
		if provider == "codex" {
			s.consume([]byte(`{"id":"cxz-quota","error":{"code":-32601,"message":"unsupported"}}`))
			s.codex.readQuota()
		} else {
			s.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-quota","subtype":"error","error":"unsupported"}}`))
			s.readClaudeQuota()
		}
		if s.snap.State != "working" || input.Len() != 0 {
			t.Fatal("telemetry failed turn or retried unsupported API")
		}
		for _, e := range s.log.All() {
			if e.Kind == "turn_end" {
				t.Fatal("quota error became failed turn")
			}
		}
	}
}

func TestClaudeTelemetryAndDefaultTools(t *testing.T) {
	args := claudeArgs()
	if slices.Contains(args, "--tools") || slices.Contains(args, "--disable-slash-commands") {
		t.Fatal(args)
	}
	if !slices.Contains(args, "--permission-mode") || !slices.Contains(args, "--setting-sources=") {
		t.Fatal("unrelated safety settings removed")
	}
	s, input := displaySupervisor(t, "claude")
	s.consume([]byte(`{"type":"control_response","response":{"request_id":"initialize","subtype":"success"}}`))
	if !bytes.Contains(input.Bytes(), []byte(`"subtype":"get_usage"`)) || !bytes.Contains(input.Bytes(), []byte(`"skip_behaviors":true`)) {
		t.Fatal("no quota query")
	}
	s.consume([]byte(`{"type":"rate_limit_event","rate_limit_info":{"rateLimitType":"five_hour","utilization":0.6}}`))
	s.consume([]byte(`{"type":"control_response","response":{"request_id":"cxz-quota","subtype":"success","response":{"rate_limits":{"seven_day":{"utilization":54}}}}}`))
	s.consume([]byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool-1","is_error":true,"content":"failed command"}]}}`))
	usage := 0
	for _, e := range s.log.All() {
		if e.Kind == "usage" {
			usage++
		}
		if e.Kind == "tool_result" && (e.RequestID != "tool-1" || !bytes.Contains(e.Payload, []byte(`"is_error":true`))) {
			t.Fatal("tool result status lost")
		}
	}
	if usage != 2 {
		t.Fatal("telemetry not normalized", usage)
	}
}
