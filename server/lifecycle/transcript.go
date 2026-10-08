package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/resource"
)

func (s SessionServer) Transcript(ctx context.Context, r *resource.SessionTranscriptRequest) (*resource.SessionTranscriptReply, error) {
	v, err := s.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.Transcript(ctx, &api.TranscriptRequest{SessionId: v.GetRuntimeId(), AfterSeq: r.GetAfterSeq(), BeforeSeq: r.GetBeforeSeq(), Limit: r.GetLimit(), SnapshotSeq: r.GetSnapshotSeq()})
	if err != nil {
		return nil, err
	}
	rows := []*resource.SessionEvent{}
	meta := []*resource.SessionEvent{}
	for _, e := range out.Events {
		rows = append(rows, eventwire.ToResource(e))
	}
	for _, e := range out.Metadata {
		meta = append(meta, eventwire.ToResource(e))
	}
	reply := resource.SessionTranscriptReply_builder{Events: rows, Metadata: meta, SnapshotSeq: &out.SnapshotSeq, HasOlder: &out.HasOlder, HasNewer: &out.HasNewer}.Build()
	if out.PrecedingInput != nil {
		reply.SetPrecedingInput(eventwire.ToResource(out.PrecedingInput))
	}
	return reply, nil
}
func (s SessionServer) EventDetails(ctx context.Context, r *resource.SessionEventDetailsRequest) (*resource.SessionEventBatch, error) {
	v, err := s.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.EventDetails(ctx, &api.EventDetailsRequest{SessionId: v.GetRuntimeId(), Seq: r.GetSeq()})
	if err != nil {
		return nil, err
	}
	rows := []*resource.SessionEvent{}
	for _, e := range out.Events {
		rows = append(rows, eventwire.ToResource(e))
	}
	return resource.SessionEventBatch_builder{Events: rows}.Build(), nil
}
