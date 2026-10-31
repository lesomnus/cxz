package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) EraseSession(ctx context.Context, r *api.SessionEraseInput, opts ...grpc.CallOption) (*api.SessionEraseReply, error) {
	if r.DryRun {
		return nil, status.Error(codes.Unimplemented, "session erase does not support dry-run")
	}
	_, err := c.sessions.Erase(ctx, sr(r.SessionId), opts...)
	if err != nil {
		return nil, err
	}
	return &api.SessionEraseReply{SessionId: r.SessionId}, nil
}

func (c *Client) GetHistoryPolicy(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.HistoryPolicy, error) {
	v, err := c.installation.Retention(ctx, resource.RetentionRequest_builder{}.Build(), opts...)
	return historyPolicy(v, err)
}

func (c *Client) SetHistoryPolicy(ctx context.Context, r *api.HistoryPolicy, opts ...grpc.CallOption) (*api.HistoryPolicy, error) {
	v, err := c.installation.SaveRetention(ctx, resource.SaveRetentionRequest_builder{
		Retention: resource.HistoryPolicy_builder{
			Disabled: &r.Disabled, MaxMib: &r.MaxMib, RawMib: &r.RawMib,
			WindowMib: &r.WindowMib, WindowTurns: &r.WindowTurns,
		}.Build(),
	}.Build(), opts...)
	return historyPolicy(v, err)
}

func historyPolicy(v *resource.HistoryPolicy, err error) (*api.HistoryPolicy, error) {
	if err != nil {
		return nil, err
	}
	return &api.HistoryPolicy{
		Disabled: v.GetDisabled(), MaxMib: v.GetMaxMib(), RawMib: v.GetRawMib(),
		WindowMib: v.GetWindowMib(), WindowTurns: v.GetWindowTurns(),
	}, nil
}
