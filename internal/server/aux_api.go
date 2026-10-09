package server

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Aux runs on the manager, not in a project. The controller that owns it holds
// the account profiles and the rolling context for every session on the
// installation, so a project runtime has neither the state nor the authority to
// answer -- which is what this refusal says.

func (s *Server) requireManagerAux() error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "auxiliary AI requires an installed host manager")
	}
	return nil
}

func (s *Server) AuxRun(ctx context.Context, r *api.AuxRunInput) (*api.AuxState, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxRun(ctx, r)
}

func (s *Server) AuxStatus(ctx context.Context, r *api.AuxStatusInput) (*api.AuxState, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxStatus(ctx, r)
}

func (s *Server) AuxPrefer(ctx context.Context, r *api.AuxPreferInput) (*api.AuxState, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxPrefer(ctx, r)
}

func (s *Server) AuxCancel(ctx context.Context, r *api.AuxCancelInput) (*api.AuxState, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxCancel(ctx, r)
}

func (s *Server) AuxForget(ctx context.Context, r *api.AuxForgetInput) (*api.Receipt, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxForget(ctx, r)
}

func (s *Server) AuxConfig(ctx context.Context, r *api.Empty) (*api.AuxConfigReply, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxConfig(ctx)
}

func (s *Server) AuxSetConfig(ctx context.Context, r *api.AuxSetConfigInput) (*api.AuxConfigReply, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxSetConfig(ctx, r)
}

func (s *Server) AuxModels(ctx context.Context, r *api.AuxModelsInput) (*api.AuxModelsReply, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxModels(ctx, r)
}

func (s *Server) AuxLoginInfo(ctx context.Context, r *api.AuxLoginInfoInput) (*api.AuxLoginInfoReply, error) {
	if err := s.requireManagerAux(); err != nil {
		return nil, err
	}
	return s.manager.AuxLoginInfo(ctx, r)
}
