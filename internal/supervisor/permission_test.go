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
func TestDefaultPermissionApprovesWithoutAnyClient(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			s, wire := displaySupervisor(t, provider)
			s.snap = Replay([]core.Event{{Kind: "state", Text: "idle", RunID: "run"}})
			tool := "Bash"
			if provider == "codex" {
				tool = "item/commandExecution/requestApproval"
			}
			s.consume(permissionRequest(provider, tool, "default"))
			if len(s.pending) != 0 || wire.Len() == 0 {
				t.Fatal("default policy required manual approval")
			}
		})
	}
}

func TestFullPermissionApprovesClaudeSearchToolsWithoutClient(t *testing.T) {
	for _, tool := range []string{"WebSearch", "WebFetch", "ToolSearch"} {
		t.Run(tool, func(t *testing.T) {
			s, wire := displaySupervisor(t, "claude")
			s.snap.PermissionMode = "ask"
			request := []byte(fmt.Sprintf(`{"type":"control_request","request_id":"search","request":{"subtype":"can_use_tool","tool_name":%q,"input":{"query":"image index verification"}}}`, tool))
			s.consume(request)
			if len(s.pending) != 1 || wire.Len() != 0 {
				t.Fatal("ask must wait for approval")
			}
			if _, err := s.execute("permission", core.Command{RunID: "run", ClientID: "enable", Text: "full"}); err != nil {
				t.Fatal(err)
			}
			if len(s.pending) != 0 || wire.Len() == 0 {
				t.Fatal("full did not resolve pending search")
			}
			var reply struct {
				Response struct {
					Response struct {
						Behavior string         `json:"behavior"`
						Input    map[string]any `json:"updatedInput"`
					} `json:"response"`
				} `json:"response"`
			}
			if err := json.Unmarshal(wire.Bytes(), &reply); err != nil {
				t.Fatal(err)
			}
			if reply.Response.Response.Behavior != "allow" || reply.Response.Response.Input["query"] != "image index verification" {
				t.Fatal("invalid tool approval", wire.String())
			}
			wire.Reset()
			s.consume(permissionRequest("claude", tool, "future"))
			if len(s.pending) != 0 || wire.Len() == 0 {
				t.Fatal("future search required a client")
			}
		})
	}
}

func TestSupervisorApprovesWithoutAnyClient(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			s, wire := displaySupervisor(t, provider)
			s.snap.PermissionMode = "ask"
			tool := "Bash"
			question := "AskUserQuestion"
			if provider == "codex" {
				tool = "item/commandExecution/requestApproval"
				question = "item/tool/requestUserInput"
			}
			s.consume(permissionRequest(provider, tool, "pending"))
			if len(s.pending) != 1 || wire.Len() != 0 {
				t.Fatal("manual mode must leave approval pending")
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
	if Replay(nil).PermissionMode != "full" {
		t.Fatal("unset policy must default to full")
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
// Full approves whatever carries the request, so a tool the provider adds later
// does not reintroduce the prompt the mode exists to remove.
func TestFullApprovesEveryToolExceptQuestions(t *testing.T) {
	for _, tool := range []string{"FutureTool", "Agent", "NotebookEdit", "mcp__server__do", ""} {
		s, wire := displaySupervisor(t, "claude")
		s.execute("permission", core.Command{RunID: "run", ClientID: "mode", Text: "full"})
		s.consume(permissionRequest("claude", tool, "any"))
		if len(s.pending) != 0 || wire.Len() == 0 {
			t.Fatal("full left a permission request pending", tool)
		}
		if s.snap.PermissionMode != "full" {
			t.Fatal("approval failure fell back to ask", tool)
		}
	}
	s, wire := displaySupervisor(t, "claude")
	s.execute("permission", core.Command{RunID: "run", ClientID: "mode", Text: "full"})
	s.consume(permissionRequest("claude", "AskUserQuestion", "question"))
	if len(s.pending) != 1 || wire.Len() != 0 {
		t.Fatal("question answered automatically")
	}
}
