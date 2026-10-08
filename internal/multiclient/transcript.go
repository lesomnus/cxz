package multiclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (c *Client) Transcript(ctx context.Context, in *api.TranscriptRequest, opts ...grpc.CallOption) (*api.TranscriptReply, error) {
	name, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	r := proto.Clone(in).(*api.TranscriptRequest)
	r.SessionId = id
	out, err := client.Transcript(ctx, r, opts...)
	if out != nil {
		out = proto.Clone(out).(*api.TranscriptReply)
		for _, rows := range [][]*api.Event{out.Events, out.Metadata} {
			for _, e := range rows {
				e.SessionId = Scope(name, e.SessionId)
			}
		}
		if out.PrecedingInput != nil {
			out.PrecedingInput.SessionId = Scope(name, out.PrecedingInput.SessionId)
		}
	}
	return out, err
}
func (c *Client) EventDetails(ctx context.Context, in *api.EventDetailsRequest, opts ...grpc.CallOption) (*api.EventBatch, error) {
	name, id, client, err := c.route(ctx, in.SessionId)
	if err != nil {
		return nil, err
	}
	r := proto.Clone(in).(*api.EventDetailsRequest)
	r.SessionId = id
	out, err := client.EventDetails(ctx, r, opts...)
	if out != nil {
		out = proto.Clone(out).(*api.EventBatch)
		for _, e := range out.Events {
			e.SessionId = Scope(name, e.SessionId)
		}
	}
	return out, err
}
