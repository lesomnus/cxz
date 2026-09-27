package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lesomnus/cxz/internal/journal"
	"os"
	"sort"
	"time"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/historypolicy"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/resource"
)

func commandDigest(c core.Command) string {
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func compactRecord(r record) record {
	if r.Digest == "" {
		r.Digest = commandDigest(r.Command)
	}
	r.Command = core.Command{ClientID: r.Command.ClientID, RunID: r.Command.RunID}
	return r
}
func resumeEvents(events []core.Event) []core.Event {
	var out []core.Event
	for _, e := range events {
		if e.Kind == core.HistoryCheckpointKind {
			var c core.HistoryCheckpoint
			_ = json.Unmarshal(e.Payload, &c)
			out = append(out, c.Resume...)
		} else {
			out = append(out, e)
		}
	}
	return out
}
func makeHistoryCheckpoint(events []core.Event) (core.Event, error) {
	if len(events) == 0 {
		return core.Event{}, fmt.Errorf("empty checkpoint")
	}
	snap := Replay(events)
	if len(snap.Pending) > 0 || (snap.State != "idle" && snap.State != "stopped" && snap.State != "failed" && snap.State != "interrupted") {
		return core.Event{}, fmt.Errorf("checkpoint is not a completed turn boundary")
	}
	latest := map[string]core.Event{}
	for _, e := range resumeEvents(events) {
		switch e.Kind {
		case "setting":
			latest["setting:"+e.Text] = e
		case "intent", "receipt", "permission":
			if e.RequestID == "" {
				continue
			}
			var r record
			if err := json.Unmarshal(e.Payload, &r); err != nil {
				return core.Event{}, err
			}
			r = compactRecord(r)
			e.Payload, _ = json.Marshal(r)
			e.Raw = nil
			e.Text = ""
			e.Kind = "receipt"
			latest["receipt:"+r.Command.ClientID] = e
		}
	}
	keys := make([]string, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c := core.HistoryCheckpoint{Version: 1, Snapshot: snap}
	for _, k := range keys {
		c.Resume = append(c.Resume, latest[k])
	}
	payload, err := json.Marshal(c)
	last := events[len(events)-1]
	return core.Event{SessionID: last.SessionID, RunID: last.RunID, Seq: last.Seq, TimeMS: last.TimeMS, Kind: core.HistoryCheckpointKind, Payload: payload}, err
}

// Called with the supervisor mutex held, only between turns. Unknown or active
// background work postpones pruning; no provider file is opened or rewritten.
func (s *Supervisor) retainHistory(root string, authorize func() error) error {
	p, err := historypolicy.Load(root)
	if err != nil {
		return err
	}
	limit, _, _ := p.Limits()
	if limit == 0 {
		return nil
	}
	size, err := s.log.Size()
	if err != nil || size <= limit {
		return err
	}
	if s.snap.State != "idle" || len(s.pending) > 0 || s.settingPending != "" || s.updateUnknown || len(s.updateTools) > 0 {
		return nil
	}
	if s.codex != nil && (s.codex.turn != "" || len(s.codex.asyncReplies) > 0) {
		return nil
	}
	for _, task := range s.updateBackground.Tasks {
		if task.Active {
			return nil
		}
	}
	events := s.log.All()
	// Pick the earliest safe boundary that leaves at most 80% of the budget.
	// The latest completed turn is retained even if it alone exceeds the budget.
	target := limit * 8 / 10
	sizes := make([]int64, len(events))
	var total int64
	lastInput := -1
	for i, e := range events {
		b, _ := json.Marshal(e)
		sizes[i] = int64(len(b) + 1)
		total += sizes[i]
		if e.Kind == "input" {
			lastInput = i
		}
	}
	if lastInput < 0 {
		lastInput = len(events) - 1
	}
	cut := -1
	pending := map[string]bool{}
	run := ""
	var background agentview.BackgroundState
	for i, e := range events {
		switch e.Kind {
		case core.HistoryCheckpointKind:
			var c core.HistoryCheckpoint
			_ = json.Unmarshal(e.Payload, &c)
			run = c.Snapshot.RunID
			for _, p := range c.Snapshot.Pending {
				pending[p.RequestID] = true
			}
		case "state":
			if run != e.RunID {
				pending = map[string]bool{}
				background = agentview.BackgroundState{}
				run = e.RunID
			}
		case "approval":
			pending[e.RequestID] = true
		case "approval_resolved":
			delete(pending, e.RequestID)
		case "raw":
			if agentview.IsBackgroundEvent(e.Raw) {
				background.Apply(e.Raw)
			}
		}

		total -= sizes[i]
		if i >= lastInput {
			break
		}
		if e.Kind == "state" && (e.Text == "idle" || e.Text == "stopped" || e.Text == "failed" || e.Text == "interrupted") && total <= target && len(pending) == 0 {
			active := false
			for _, task := range background.Tasks {
				active = active || task.Active
			}
			if active {
				continue
			}
			cut = i
			break
		}
	}
	if cut < 0 {
		return nil
	}
	checkpoint, err := makeHistoryCheckpoint(events[:cut+1])
	if err != nil {
		return err
	}
	if authorize != nil {
		if err = authorize(); err != nil {
			return err
		}
	}
	if err = s.log.Compact(checkpoint); err != nil {
		var commit *journal.CompactionCommitError
		if errors.As(err, &commit) {
			s.kill()
			panic(err)
		}
		return err
	}
	// Release large command bodies in memory without forgetting duplicate IDs.
	for _, e := range resumeEvents([]core.Event{checkpoint}) {
		if e.Kind == "receipt" {
			var r record
			_ = json.Unmarshal(e.Payload, &r)
			if current, ok := s.receipts[r.Command.ClientID]; ok && current.Status == r.Status {
				s.receipts[r.Command.ClientID] = r
			}
		}
	}
	s.event(core.HistoryTrimmedKind, "Earlier display history was removed by the size limit.", "", core.HistoryBoundary{Through: checkpoint.Seq}, nil)
	return nil
}
func authorizeHistoryCheckpoint(root string) error {
	conn, err := transport.Dial(root)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	action := "history-checkpoint-ready"
	_, err = resource.NewProjectServiceClient(conn).Docker(ctx, resource.DockerRequest_builder{Action: &action}.Build())
	return err
}

// Called under mu after completed turns and on the periodic idle check.
func (s *Supervisor) maintainHistory(root string) {
	if err := s.retainHistory(root, func() error { return authorizeHistoryCheckpoint(root) }); err != nil {
		fmt.Fprintln(os.Stderr, "history retention:", err)
	}
}
