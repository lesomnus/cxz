package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
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

// Rows is Transcript streamed, so it resolves the session the same way and then
// gets out of the way: no snapshot refresh on the way through, unlike Events.
// Events carries the records a session's state is derived from, so that stream
// keeps the session row it is paired with current; rows are already derived.
func (s SessionServer) Rows(r *resource.SessionRowsRequest, stream grpc.ServerStreamingServer[resource.SessionRow]) error {
	if err := s.effect(); err != nil {
		return err
	}
	v, err := s.Get(stream.Context(), resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	return s.shared.runtime.Rows(&api.RowsRequest{SessionId: v.GetRuntimeId(), AfterUpdated: r.GetAfterUpdated(), ClientId: r.GetClientId()}, rowStream{ServerStreamingServer: stream})
}

type rowStream struct {
	grpc.ServerStreamingServer[resource.SessionRow]
}

func (s rowStream) Send(v *api.Row) error {
	out := resource.SessionRow_builder{Updated: &v.Updated}.Build()
	if v.Event != nil {
		out.SetEvent(eventwire.ToResource(v.Event))
	}
	return s.ServerStreamingServer.Send(out)
}
