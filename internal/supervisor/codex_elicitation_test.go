package supervisor

import (
	"encoding/json"
	"github.com/lesomnus/cxz/internal/core"
	"testing"
)

const memoryElicitation = `{"id":"mcp-1","method":"mcpServer/elicitation/request","params":{"threadId":"thread","serverName":"cxz_memory","mode":"form","message":"Allow memory_list?","requestedSchema":{"type":"object","properties":{}}}}`

func TestCodexMCPElicitationRequiresExplicitResponse(t *testing.T) {
	for _, mode := range []string{"ask", "full"} {
		for _, action := range []string{"Accept", "Decline", "Cancel"} {
			t.Run(mode+action, func(t *testing.T) {
				s, input := displaySupervisor(t, "codex")
				s.snap.PermissionMode = mode
				s.consume([]byte(memoryElicitation))
				if len(s.pending) != 1 || input.Len() != 0 || s.updateUnknown || s.snap.State != "waiting_input" {
					t.Fatal("elicitation rejected or auto-answered", input.String())
				}
				var id string
				for k := range s.pending {
					id = k
				}
				c := core.Command{RunID: "run", ClientID: "answer", RequestID: id, Allow: true, Selections: map[string]core.AnswerSelection{"cxz:action": {Selected: []string{action}}}}
				if _, err := s.execute("reply", c); err != nil {
					t.Fatal(err)
				}
				var wire struct {
					ID     string
					Result struct {
						Action  string
						Content any
					}
					Error any
				}
				if err := json.Unmarshal(input.Bytes(), &wire); err != nil {
					t.Fatal(err)
				}
				if wire.ID != "mcp-1" || wire.Error != nil || wire.Result.Action == "" || len(s.pending) != 0 {
					t.Fatal(input.String())
				}
				if (wire.Result.Content != nil) != (action == "Accept") {
					t.Fatal(input.String())
				}
				wantResolved := map[string]string{"Accept": "allowed", "Decline": "denied", "Cancel": "canceled"}[action]
				found := false
				for _, event := range s.log.All() {
					if event.Kind == "approval_resolved" {
						found = true
						if event.Text != wantResolved {
							t.Fatalf("wrong resolution: %s", event.Text)
						}
					}
				}
				if !found {
					t.Fatal("missing durable resolution")
				}
				n := input.Len()
				if _, err := s.execute("reply", c); err != nil {
					t.Fatal(err)
				}
				if input.Len() != n {
					t.Fatal("duplicate reply")
				}
			})
		}
	}
}
func TestCodexElicitationInvalidAnswerStaysPending(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.consume([]byte(memoryElicitation))
	var id string
	for k := range s.pending {
		id = k
	}
	if _, err := s.execute("reply", core.Command{RunID: "run", ClientID: "bad", RequestID: id, Allow: true}); err == nil {
		t.Fatal("accepted missing action")
	}
	if len(s.pending) != 1 || input.Len() != 0 {
		t.Fatal("invalid answer changed request")
	}
	if _, err := s.execute("reply", core.Command{RunID: "run", ClientID: "no", RequestID: id, Allow: false}); err != nil {
		t.Fatal(err)
	}
	if len(s.pending) != 0 {
		t.Fatal("decline did not clear pending")
	}
}

func TestCodexElicitationServerResolution(t *testing.T) {
	s, input := displaySupervisor(t, "codex")
	s.consume([]byte(memoryElicitation))
	s.consume([]byte(`{"method":"serverRequest/resolved","params":{"threadId":"thread","requestId":"mcp-1"}}`))
	if len(s.pending) != 0 || s.snap.State == "waiting_input" || input.Len() != 0 {
		t.Fatal("resolved prompt still pending")
	}
	if _, err := s.execute("reply", core.Command{RunID: "run", ClientID: "late", RequestID: `"mcp-1"`, Allow: true}); err == nil {
		t.Fatal("stale request accepted")
	}
}
