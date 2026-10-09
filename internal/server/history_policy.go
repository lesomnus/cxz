package server

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/historypolicy"
)

// A project runtime keeps the budgets it was given and applies them to its own
// journals. With a manager above it the manager owns the file and pushes it
// here, which is why both layers answer the same two calls.

func (s *Server) GetHistoryPolicy(ctx context.Context, in *api.Empty) (*api.HistoryPolicy, error) {
	if s.manager != nil {
		return s.manager.GetHistoryPolicy(ctx, in)
	}
	p, err := historypolicy.Load(s.root)
	if err != nil {
		return nil, err
	}
	return runtimeHistoryPolicy(p), nil
}

func (s *Server) SetHistoryPolicy(ctx context.Context, in *api.HistoryPolicy) (*api.HistoryPolicy, error) {
	if s.manager != nil {
		return s.manager.SetHistoryPolicy(ctx, in)
	}
	p := historypolicy.Policy{
		Disabled: in.Disabled, MaxMiB: int(in.MaxMib), RawMiB: int(in.RawMib),
		WindowMiB: int(in.WindowMib), WindowTurns: int(in.WindowTurns),
	}
	if err := historypolicy.Save(s.root, p); err != nil {
		return nil, err
	}
	return s.GetHistoryPolicy(ctx, &api.Empty{})
}

func runtimeHistoryPolicy(p historypolicy.Policy) *api.HistoryPolicy {
	return &api.HistoryPolicy{
		Disabled: p.Disabled, MaxMib: int32(p.MaxMiB), RawMib: int32(p.RawMiB),
		WindowMib: int32(p.WindowMiB), WindowTurns: int32(p.WindowTurns),
	}
}

// MarkHistoryTrimmable records that this projection may now hold trimmed
// history. A release that predates trimming reads the version and refuses
// rather than serving a conversation that is no longer there, which is why the
// supervisor asks before it compacts and stops if the answer is an error.
func (s *Server) MarkHistoryTrimmable(ctx context.Context, _ *api.Empty) (*api.Empty, error) {
	if _, err := s.db.ExecContext(ctx, "PRAGMA user_version=2"); err != nil {
		return nil, err
	}
	return &api.Empty{}, nil
}

// GetHistoryFloor says how far this runtime has trimmed. Only the manager
// asks, so that its cache stops offering history the journal no longer has;
// there is no client-facing call for it.
func (s *Server) GetHistoryFloor(ctx context.Context, r *api.SessionRef) (*api.HistoryFloorReply, error) {
	manifest, err := s.manifest(ctx, r.Id)
	if err != nil {
		return nil, err
	}
	p, err := s.lockProjection(ctx, manifest)
	if err != nil {
		return nil, err
	}
	through := p.floor
	p.mu.Unlock()
	return &api.HistoryFloorReply{Through: through}, nil
}
