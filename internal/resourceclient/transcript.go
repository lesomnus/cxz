package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/eventwire"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) Transcript(ctx context.Context, r *api.TranscriptRequest, opts ...grpc.CallOption) (*api.TranscriptReply, error) {
	v, err := c.sessions.Transcript(ctx, resource.SessionTranscriptRequest_builder{Ref: sr(r.SessionId), AfterSeq: &r.AfterSeq, BeforeSeq: &r.BeforeSeq, Limit: &r.Limit, SnapshotSeq: &r.SnapshotSeq}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	out := &api.TranscriptReply{SnapshotSeq: v.GetSnapshotSeq(), HasOlder: v.GetHasOlder(), HasNewer: v.GetHasNewer()}
	for _, e := range v.GetEvents() {
		out.Events = append(out.Events, eventwire.FromResource(r.SessionId, e))
	}
	for _, e := range v.GetMetadata() {
		out.Metadata = append(out.Metadata, eventwire.FromResource(r.SessionId, e))
	}
	if v.GetPrecedingInput() != nil {
		out.PrecedingInput = eventwire.FromResource(r.SessionId, v.GetPrecedingInput())
	}
	return out, nil
}
func (c *Client) EventDetails(ctx context.Context, r *api.EventDetailsRequest, opts ...grpc.CallOption) (*api.EventBatch, error) {
	v, err := c.sessions.EventDetails(ctx, resource.SessionEventDetailsRequest_builder{Ref: sr(r.SessionId), Seq: &r.Seq}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	out := &api.EventBatch{}
	for _, e := range v.GetEvents() {
		out.Events = append(out.Events, eventwire.FromResource(r.SessionId, e))
	}
	return out, nil
}
