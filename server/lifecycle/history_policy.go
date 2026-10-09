package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
)

// The budgets belong to the installation, so they are read and written here
// rather than per session. They bound cxz's own records only; provider context
// is untouched. See docs/history.md.

func (s ProjectServer) MarkHistoryTrimmable(ctx context.Context, _ *resource.HistoryTrimmableRequest) (*resource.HistoryTrimmableReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	if _, err := s.shared.runtime.MarkHistoryTrimmable(ctx, &api.Empty{}); err != nil {
		return nil, err
	}
	return resource.HistoryTrimmableReply_builder{}.Build(), nil
}

func (s ProjectServer) GetHistoryPolicy(ctx context.Context, _ *resource.HistoryPolicyRequest) (*resource.HistoryPolicy, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.GetHistoryPolicy(ctx, &api.Empty{})
	return historyPolicy(out, err)
}

func (s ProjectServer) SetHistoryPolicy(ctx context.Context, r *resource.HistoryPolicy) (*resource.HistoryPolicy, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.SetHistoryPolicy(ctx, &api.HistoryPolicy{
		Disabled: r.GetDisabled(), MaxMib: r.GetMaxMib(), RawMib: r.GetRawMib(),
		WindowMib: r.GetWindowMib(), WindowTurns: r.GetWindowTurns(),
	})
	return historyPolicy(out, err)
}

func historyPolicy(v *api.HistoryPolicy, err error) (*resource.HistoryPolicy, error) {
	if err != nil {
		return nil, err
	}
	return resource.HistoryPolicy_builder{
		Disabled: &v.Disabled, MaxMib: &v.MaxMib, RawMib: &v.RawMib,
		WindowMib: &v.WindowMib, WindowTurns: &v.WindowTurns,
	}.Build(), nil
}
