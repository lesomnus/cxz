package supervisor

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/lesomnus/cxz/internal/core"
)

func permissionRequest(provider, name, id string) []byte {
	if provider == "claude" {
		return []byte(fmt.Sprintf(`{"type":"control_request","request_id":%q,"request":{"subtype":"can_use_tool","tool_name":%q,"input":{"command":"pwd"}}}`, id, name))
	}
	return []byte(fmt.Sprintf(`{"id":%q,"method":%q,"params":{"itemId":"tool","permissions":{"network":{"enabled":true}}}}`, id, name))
}
func TestSupervisorApprovesWithoutAnyClient(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			s, wire := displaySupervisor(t, provider)
			tool := "Bash"
			question := "AskUserQuestion"
			if provider == "codex" {
				tool = "item/commandExecution/requestApproval"
				question = "item/tool/requestUserInput"
			}
			s.consume(permissionRequest(provider, tool, "pending"))
			if len(s.pending) != 1 || wire.Len() != 0 {
				t.Fatal("default must be manual")
			}
			command := core.Command{RunID: "run", ClientID: "enable", Text: "full"}
			if r, err := s.execute("permission", command); err != nil || r.Status != "accepted" {
				t.Fatal(r, err)
			}
			if len(s.pending) != 0 || wire.Len() == 0 || s.snap.PermissionMode != "full" {
				t.Fatal("existing approval not handled by supervisor")
			}
			wire.Reset()
			s.consume(permissionRequest(provider, tool, "future"))
			if len(s.pending) != 0 || wire.Len() == 0 {
				t.Fatal("future approval requires a client")
			}
			var reply map[string]json.RawMessage
			if json.Unmarshal(wire.Bytes(), &reply) != nil {
				t.Fatal(wire.String())
			}
			wire.Reset()
			s.consume(permissionRequest(provider, question, "question"))
			if len(s.pending) != 1 || wire.Len() != 0 {
				t.Fatal("question was answered automatically")
			}
			before := len(s.log.All())
			if _, err := s.execute("permission", command); err != nil || len(s.log.All()) != before {
				t.Fatal("policy retry was not idempotent", err)
			}
			if _, err := s.execute("permission", core.Command{RunID: "run", ClientID: "disable", Text: "ask"}); err != nil {
				t.Fatal(err)
			}
			s.consume(permissionRequest(provider, tool, "manual"))
			if len(s.pending) != 2 || wire.Len() != 0 {
				t.Fatal("ask did not stop automatic approval")
			}
			if _, err := s.execute("permission", command); err != nil || s.snap.PermissionMode != "ask" {
				t.Fatal("old enable retry undid a later ask", err)
			}
		})
	}
}
func TestPermissionValidationAndReplay(t *testing.T) {
	s, wire := displaySupervisor(t, "claude")
	for _, c := range []core.Command{{RunID: "old", ClientID: "stale", Text: "full"}, {RunID: "run", ClientID: "invalid", Text: "yes"}, {RunID: "run", Text: "full"}} {
		if _, err := s.execute("permission", c); err == nil {
			t.Fatal("invalid permission accepted")
		}
	}
	if wire.Len() != 0 {
		t.Fatal("permission sent as a provider prompt")
	}
	s.execute("permission", core.Command{RunID: "run", ClientID: "one", Text: "full"})
	s.event("state", "stopped", "", nil, nil)
	s.event("state", "starting", "", nil, nil)
	restored := Replay(s.log.All())
	if restored.PermissionMode != "full" {
		t.Fatal("restart lost policy")
	}
	if Replay(nil).PermissionMode != "ask" {
		t.Fatal("legacy session must default to ask")
	}
	s.snap.State = "idle"
	s.execute("permission", core.Command{RunID: "run", ClientID: "two", Text: "ask"})
	if Replay(s.log.All()).PermissionMode != "ask" {
		t.Fatal("manual policy not durable")
	}
	if _, err := s.execute("permission", core.Command{RunID: "run", ClientID: "two", Text: "full"}); err == nil {
		t.Fatal("conflicting request ID accepted")
	}
}
func TestAutomaticDecisionFollowsDurableRequestAndPolicy(t *testing.T) {
	s, wire := displaySupervisor(t, "claude")
	s.execute("permission", core.Command{RunID: "run", ClientID: "mode", Text: "full"})
	s.consume(permissionRequest("claude", "Write", "write"))
	var policy, request, intent, resolved, receipt uint64
	for _, e := range s.log.All() {
		switch e.Kind {
		case "permission":
			policy = e.Seq
		case "approval":
			request = e.Seq
		case "intent":
			intent = e.Seq
		case "approval_resolved":
			resolved = e.Seq
		case "receipt":
			receipt = e.Seq
		}
	}
	if !(policy < request && request < intent && intent < resolved && resolved < receipt) || wire.Len() == 0 {
		t.Fatal("decision journal order", policy, request, intent, resolved, receipt)
	}
}
func TestFullDoesNotApproveUnknownClaudeTool(t *testing.T) {
	s, wire := displaySupervisor(t, "claude")
	s.execute("permission", core.Command{RunID: "run", ClientID: "mode", Text: "full"})
	s.consume(permissionRequest("claude", "FutureTool", "unknown"))
	if len(s.pending) != 1 || wire.Len() != 0 {
		t.Fatal("unknown tool approved")
	}
}
