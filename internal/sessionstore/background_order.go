package sessionstore

import (
	"context"
	"encoding/json"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventview"
)

// Startup already reads these events. Recover timing, ordering and legacy Codex
// command fields once, without replaying history on every Background RPC.
func (s *Store) backfillBackgroundOrder(ctx context.Context, events []core.Event) error {
	version, err := s.number(ctx, "background_order")
	if err != nil || version >= 3 {
		return err
	}
	if err = s.loadBackground(ctx); err != nil {
		return err
	}
	replay := map[string]*agentview.BackgroundState{}
	var visit func([]core.Event)
	visit = func(events []core.Event) {
		for _, e := range events {
			if e.Kind == core.HistoryCheckpointKind {
				var checkpoint core.HistoryCheckpoint
				if json.Unmarshal(e.Payload, &checkpoint) == nil {
					visit(checkpoint.Resume)
				}
				continue
			}
			if e.Kind != "background" && e.Kind != "raw" {
				continue
			}
			v := eventview.FromCore(e)
			if v.Kind != "background" {
				continue
			}
			if replay[e.RunID] == nil {
				replay[e.RunID] = &agentview.BackgroundState{}
			}
			replay[e.RunID].ApplyAt(v.Payload, e.Seq, e.TimeMS)
		}
	}
	visit(events)
	for run, state := range s.background {
		if state == nil || replay[run] == nil {
			continue
		}
		for id, task := range state.Tasks {
			recovered, ok := replay[run].Tasks[id]
			// Recover the old Codex field mapping only when the retained
			// provider event proves this description was a terminal command.
			if ok && task.Command == "" && recovered.Command != "" && recovered.Description == "" && task.Description == recovered.Command {
				task.Command, task.Description = recovered.Command, ""
				state.Tasks[id] = task
			}
			if task.UpdatedSeq != 0 && task.StartedAtMS != 0 && (task.Active || task.FinishedAtMS != 0) {
				continue
			}
			// A compacted or shed record cannot tell us when an unseen task changed.
			before := recovered
			before.UpdatedSeq, before.UpdatedAtMS = 0, 0
			before.StartedAtMS, before.FinishedAtMS = 0, 0
			current := task
			current.UpdatedSeq, current.UpdatedAtMS = 0, 0
			current.StartedAtMS, current.FinishedAtMS = 0, 0
			if !ok || before != current {
				continue
			}
			if task.UpdatedSeq == 0 {
				task.UpdatedSeq, task.UpdatedAtMS = recovered.UpdatedSeq, recovered.UpdatedAtMS
			}
			if task.StartedAtMS == 0 {
				task.StartedAtMS = recovered.StartedAtMS
			}
			if !task.Active && task.FinishedAtMS == 0 {
				task.FinishedAtMS = recovered.FinishedAtMS
			}
			state.Tasks[id] = task
		}
	}
	raw, err := json.Marshal(s.background)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('background',?)`, string(raw)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('background_order','3')`); err != nil {
		return err
	}
	return tx.Commit()
}
