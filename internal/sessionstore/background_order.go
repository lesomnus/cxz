package sessionstore

import (
	"context"
	"encoding/json"

	"github.com/lesomnus/cxz/internal/agentview"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/eventview"
)

// Startup already reads these events. Recover old ordering once, without
// replaying conversation history on every Background RPC.
func (s *Store) backfillBackgroundOrder(ctx context.Context, events []core.Event) error {
	version, err := s.number(ctx, "background_order")
	if err != nil || version >= 1 {
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
			if task.UpdatedSeq != 0 {
				continue
			}
			recovered, ok := replay[run].Tasks[id]
			// A compacted or shed record cannot tell us when an unseen task changed.
			before := recovered
			before.UpdatedSeq, before.UpdatedAtMS = 0, 0
			current := task
			current.UpdatedSeq, current.UpdatedAtMS = 0, 0
			if !ok || before != current {
				continue
			}
			task.UpdatedSeq, task.UpdatedAtMS = recovered.UpdatedSeq, recovered.UpdatedAtMS
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
	if _, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO meta VALUES('background_order','1')`); err != nil {
		return err
	}
	return tx.Commit()
}
