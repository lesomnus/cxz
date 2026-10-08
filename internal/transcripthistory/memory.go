package transcripthistory

import (
	"slices"
	"strings"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FromEvents implements the same reading projection for the in-memory WASM
// sandbox. Production uses the indexed SQL queries, not a journal scan per page.
func FromEvents(events []*api.Event, r *api.TranscriptRequest) (*api.TranscriptReply, error) {
	if r.AfterSeq > 0 && r.BeforeSeq > 0 {
		return nil, status.Error(codes.InvalidArgument, "before_seq and after_seq are mutually exclusive")
	}
	out := &api.TranscriptReply{SnapshotSeq: r.SnapshotSeq}
	if out.SnapshotSeq == 0 {
		for _, e := range events {
			out.SnapshotSeq = max(out.SnapshotSeq, e.Seq)
		}
	}
	projection := New()
	for _, e := range events {
		if e.Seq <= out.SnapshotSeq {
			projection.Apply(e)
			if slices.Contains([]string{"models", "usage", "compact", "turn_end"}, e.Kind) {
				if v := metadata(e); v != nil {
					out.Metadata = append(out.Metadata, v)
				}
			}
		}
	}
	if len(out.Metadata) > 64 {
		out.Metadata = out.Metadata[len(out.Metadata)-64:]
	}
	all := make([]*api.Event, 0, len(projection.Rows))
	for _, e := range projection.Rows {
		all = append(all, e)
	}
	slices.SortFunc(all, func(a, b *api.Event) int {
		if a.Seq < b.Seq {
			return -1
		}
		if a.Seq > b.Seq {
			return 1
		}
		return 0
	})
	for _, e := range all {
		if (r.AfterSeq == 0 || e.Seq > r.AfterSeq) && (r.BeforeSeq == 0 || e.Seq < r.BeforeSeq) {
			out.Events = append(out.Events, e)
		}
	}
	limit := pageLimit(r.Limit)
	if len(out.Events) > limit {
		if r.AfterSeq > 0 {
			out.Events = out.Events[:limit]
		} else {
			out.Events = out.Events[len(out.Events)-limit:]
		}
	}
	if len(out.Events) > 0 {
		first, last := out.Events[0].Seq, out.Events[len(out.Events)-1].Seq
		for _, e := range all {
			if e.Seq < first {
				out.HasOlder = true
				if e.Kind == "input" {
					out.PrecedingInput = e
				}
			}
			if e.Seq > last {
				out.HasNewer = true
			}
		}
	}
	return out, nil
}
func DetailsFromEvents(events []*api.Event, seq uint64) (*api.EventBatch, error) {
	var anchor *api.Event
	for _, e := range events {
		if e.Seq == seq {
			anchor = e
			break
		}
	}
	if anchor == nil {
		return nil, status.Error(codes.NotFound, "event is no longer retained")
	}
	if !strings.HasPrefix(anchor.Kind, "tool_") || anchor.RequestId == "" {
		return &api.EventBatch{Events: []*api.Event{anchor}}, nil
	}
	approvals := map[string]bool{}
	for _, e := range events {
		if e.RunId == anchor.RunId && e.Kind == "approval" && related(e) == anchor.RequestId {
			approvals[e.RequestId] = true
		}
	}
	out := &api.EventBatch{}
	for _, e := range events {
		if e.RunId == anchor.RunId && e.Kind != "raw" && (e.RequestId == anchor.RequestId || related(e) == anchor.RequestId || approvals[e.RequestId]) {
			out.Events = append(out.Events, e)
		}
	}
	return out, nil
}
