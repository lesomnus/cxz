package server

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The shared Docker engine belongs to the host, which a project runtime is
// inside of rather than on. Each operation refuses by itself now; one handler
// used to refuse the whole envelope, so the reason was the same sentence
// whatever had been asked.
func (s *Server) requireManagerEngine() error {
	if s.manager == nil {
		return status.Error(codes.FailedPrecondition, "managed Docker requires an installed host manager")
	}
	return nil
}

func (s *Server) SaveEngine(ctx context.Context, r *api.SaveEngineInput) (*api.EngineReply, error) {
	if err := s.requireManagerEngine(); err != nil {
		return nil, err
	}
	return s.manager.SaveEngine(ctx, r)
}

func (s *Server) StartEngine(ctx context.Context, r *api.StartEngineInput) (*api.EngineReply, error) {
	if err := s.requireManagerEngine(); err != nil {
		return nil, err
	}
	return s.manager.StartEngine(ctx, r)
}

func (s *Server) StopEngine(ctx context.Context, r *api.Empty) (*api.EngineReply, error) {
	if err := s.requireManagerEngine(); err != nil {
		return nil, err
	}
	return s.manager.StopEngine(ctx, r)
}

func (s *Server) PruneEngine(ctx context.Context, r *api.Empty) (*api.EngineReply, error) {
	if err := s.requireManagerEngine(); err != nil {
		return nil, err
	}
	return s.manager.PruneEngine(ctx, r)
}

func (s *Server) EngineStatus(ctx context.Context, r *api.Empty) (*api.EngineReply, error) {
	if err := s.requireManagerEngine(); err != nil {
		return nil, err
	}
	return s.manager.EngineStatus(ctx, r)
}

func (s *Server) GetEngineInfo(ctx context.Context, r *api.Empty) (*api.EngineInfo, error) {
	if err := s.requireManagerEngine(); err != nil {
		return nil, err
	}
	return s.manager.GetEngineInfo(ctx, r)
}

// What cxz is, a project runtime does know: it is the same build, installed by
// the same manager. But the channel and the pin are the installation's files,
// which only the manager has.
func (s *Server) GetInstallationVersion(ctx context.Context, r *api.Empty) (*api.InstallationVersion, error) {
	if err := s.requireManagerEngine(); err != nil {
		return nil, err
	}
	return s.manager.GetInstallationVersion(ctx, r)
}
