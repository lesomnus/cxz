package supervisor

import (
	"fmt"
	"sort"

	"github.com/lesomnus/cxz/internal/core"
)

func (s *Supervisor) setPermission(c core.Command) (core.Receipt, error) {
	out := core.Receipt{ClientID: c.ClientID}
	if c.Text != "ask" && c.Text != "full" {
		return out, fmt.Errorf("permission mode must be ask or full")
	}
	if s.snap.State != "idle" && s.snap.State != "working" && s.snap.State != "waiting_input" {
		return out, fmt.Errorf("permission changes require a running session")
	}
	// One durable event is both the policy and the idempotent receipt. Recovery
	// cannot restore an accepted receipt without restoring the associated policy.
	rec := record{Op: "permission", Command: c, Status: "accepted"}
	s.event("permission", c.Text, c.ClientID, rec, nil)
	s.receipts[c.ClientID] = rec
	s.approvePending()
	out.Status = "accepted"
	return out, nil
}

// Called under mu, only after the incoming provider batch is durable. The same
// reply path handles manual and automatic decisions, including delivery-unknown
// semantics. No client, active watch stream, or TUI is needed.
func (s *Supervisor) approvePending() {
	if s.snap.PermissionMode != "full" || s.stopping || (s.snap.State != "idle" && s.snap.State != "working" && s.snap.State != "waiting_input") {
		return
	}
	var pending []core.Event
	for _, p := range s.pending {
		if p.RunID == s.snap.RunID && core.AutomaticApproval(p.Text) {
			pending = append(pending, p)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Seq < pending[j].Seq })
	for _, p := range pending {
		// A provider may repeat a request ID. Never replay a previous write whose
		// delivery was uncertain, or turn a repeated notification into a second reply.
		id := "auto:" + s.snap.RunID + ":" + p.RequestID
		if _, exists := s.receipts[id]; exists {
			continue
		}
		receipt, err := s.executeLocked("reply", core.Command{RunID: s.snap.RunID, ClientID: id, RequestID: p.RequestID, Allow: true})
		if err != nil {
			// Keep malformed requests visible instead of silently hiding them in full mode.
			s.event("permission", "ask", "", nil, nil)
			s.event("diagnostic", "automatic approval failed; manual approval enabled", "", nil, nil)
			return
		}
		if receipt.Status != "accepted" {
			return
		}
	}
}
