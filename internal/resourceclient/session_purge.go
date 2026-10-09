package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) PurgeSession(ctx context.Context, r *api.SessionPurgeInput, opts ...grpc.CallOption) (*api.SessionPurgeReply, error) {
	v, err := c.sessions.Purge(ctx, resource.SessionPurgeRequest_builder{
		Ref: sr(r.SessionId), DryRun: &r.DryRun,
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	out := &api.SessionPurgeReply{
		SessionId: r.SessionId, DryRun: v.GetDryRun(), Retained: v.GetRetained(),
	}
	for _, t := range v.GetTargets() {
		out.Targets = append(out.Targets, &api.SessionPurgeTarget{
			Kind: t.GetKind(), Path: t.GetPath(), Files: t.GetFiles(), Bytes: t.GetBytes(),
		})
	}
	return out, nil
}

func (c *Client) MarkHistoryTrimmable(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.Empty, error) {
	if _, err := c.projects.MarkHistoryTrimmable(ctx, resource.HistoryTrimmableRequest_builder{}.Build(), opts...); err != nil {
		return nil, err
	}
	return &api.Empty{}, nil
}

func (c *Client) GetHistoryPolicy(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.HistoryPolicy, error) {
	v, err := c.projects.GetHistoryPolicy(ctx, resource.HistoryPolicyRequest_builder{}.Build(), opts...)
	return historyPolicy(v, err)
}

func (c *Client) SetHistoryPolicy(ctx context.Context, r *api.HistoryPolicy, opts ...grpc.CallOption) (*api.HistoryPolicy, error) {
	v, err := c.projects.SetHistoryPolicy(ctx, resource.HistoryPolicy_builder{
		Disabled: &r.Disabled, MaxMib: &r.MaxMib, RawMib: &r.RawMib,
		WindowMib: &r.WindowMib, WindowTurns: &r.WindowTurns,
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
