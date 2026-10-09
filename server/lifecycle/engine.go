package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/enginemode"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The shared Docker engine, one method per operation. The mode is an enum
// because it has two values; the operation is a method because it is an
// operation.

func (s ProjectServer) SaveEngine(ctx context.Context, r *resource.SaveEngineRequest) (*resource.EngineReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	spec, err := runtimeEngineSpec(r.GetSpec(), true)
	if err != nil {
		return nil, err
	}
	return engineReply(s.shared.runtime.SaveEngine(ctx, &api.SaveEngineInput{Spec: spec}))
}

func (s ProjectServer) StartEngine(ctx context.Context, r *resource.StartEngineRequest) (*resource.EngineReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	spec, err := runtimeEngineSpec(r.GetSpec(), false)
	if err != nil {
		return nil, err
	}
	return engineReply(s.shared.runtime.StartEngine(ctx, &api.StartEngineInput{Spec: spec}))
}

func (s ProjectServer) StopEngine(ctx context.Context, _ *resource.EngineRequest) (*resource.EngineReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return engineReply(s.shared.runtime.StopEngine(ctx, &api.Empty{}))
}

func (s ProjectServer) PruneEngine(ctx context.Context, _ *resource.EngineRequest) (*resource.EngineReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return engineReply(s.shared.runtime.PruneEngine(ctx, &api.Empty{}))
}

func (s ProjectServer) EngineStatus(ctx context.Context, _ *resource.EngineRequest) (*resource.EngineReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	return engineReply(s.shared.runtime.EngineStatus(ctx, &api.Empty{}))
}

func (s ProjectServer) GetEngineInfo(ctx context.Context, _ *resource.EngineRequest) (*resource.EngineInfo, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.GetEngineInfo(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	mode := enginemode.Of(out.Mode)
	return resource.EngineInfo_builder{
		Mode: &mode, State: &out.State, Health: &out.Health,
		Image: &out.Image, ConfiguredImage: &out.ConfiguredImage, Endpoint: &out.Endpoint,
		BuildCache: &out.BuildCache, Reclaimable: &out.Reclaimable, UsageError: &out.UsageError,
	}.Build(), nil
}

func (s ProjectServer) GetInstallationVersion(ctx context.Context, _ *resource.EngineRequest) (*resource.InstallationVersion, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.GetInstallationVersion(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	return resource.InstallationVersion_builder{
		Version: &out.Version, Revision: &out.Revision, Channel: &out.Channel,
		Pin: &out.Pin, Error: &out.Error,
	}.Build(), nil
}

// A manager asks a project runtime how far it has trimmed, over this API,
// because this is what the connection between them speaks.
func (s SessionServer) GetHistoryFloor(ctx context.Context, r *resource.SessionHistoryFloorRequest) (*resource.SessionHistoryFloorReply, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.GetHistoryFloor(ctx, &api.SessionRef{Id: v.GetRuntimeId()})
	if err != nil {
		return nil, err
	}
	return resource.SessionHistoryFloorReply_builder{Through: &out.Through}.Build(), nil
}

// runtimeEngineSpec refuses a mode this build does not know rather than saving
// a configuration that means something else. required says whether an absent
// spec is allowed: a start may fall back on the saved one, a save may not.
func runtimeEngineSpec(v *resource.EngineSpec, required bool) (*api.EngineSpec, error) {
	if v == nil {
		if required {
			return nil, status.Error(codes.InvalidArgument, "an engine configuration is required to save one")
		}
		return nil, nil
	}
	mode := enginemode.Name(v.GetMode())
	if mode == "" {
		return nil, status.Error(codes.InvalidArgument, "unknown engine mode")
	}
	return &api.EngineSpec{Mode: mode, Image: v.GetImage(), Override: v.GetOverride()}, nil
}

func engineReply(v *api.EngineReply, err error) (*resource.EngineReply, error) {
	if err != nil {
		return nil, err
	}
	return resource.EngineReply_builder{Status: &v.Status}.Build(), nil
}
