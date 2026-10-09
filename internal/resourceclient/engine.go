package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/enginemode"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) SaveEngine(ctx context.Context, r *api.SaveEngineInput, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return engineReply(c.projects.SaveEngine(ctx, resource.SaveEngineRequest_builder{
		Spec: resourceEngineSpec(r.Spec),
	}.Build(), opts...))
}

func (c *Client) StartEngine(ctx context.Context, r *api.StartEngineInput, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return engineReply(c.projects.StartEngine(ctx, resource.StartEngineRequest_builder{
		Spec: resourceEngineSpec(r.Spec),
	}.Build(), opts...))
}

func (c *Client) StopEngine(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return engineReply(c.projects.StopEngine(ctx, resource.EngineRequest_builder{}.Build(), opts...))
}

func (c *Client) PruneEngine(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return engineReply(c.projects.PruneEngine(ctx, resource.EngineRequest_builder{}.Build(), opts...))
}

func (c *Client) EngineStatus(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.EngineReply, error) {
	return engineReply(c.projects.EngineStatus(ctx, resource.EngineRequest_builder{}.Build(), opts...))
}

func (c *Client) GetEngineInfo(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.EngineInfo, error) {
	v, err := c.projects.GetEngineInfo(ctx, resource.EngineRequest_builder{}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.EngineInfo{
		Mode: enginemode.Name(v.GetMode()), State: v.GetState(), Health: v.GetHealth(),
		Image: v.GetImage(), ConfiguredImage: v.GetConfiguredImage(), Endpoint: v.GetEndpoint(),
		BuildCache: v.GetBuildCache(), Reclaimable: v.GetReclaimable(), UsageError: v.GetUsageError(),
	}, nil
}

func (c *Client) GetInstallationVersion(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.InstallationVersion, error) {
	v, err := c.projects.GetInstallationVersion(ctx, resource.EngineRequest_builder{}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.InstallationVersion{
		Version: v.GetVersion(), Revision: v.GetRevision(), Channel: v.GetChannel(),
		Pin: v.GetPin(), Error: v.GetError(),
	}, nil
}

// GetHistoryFloor is on this API after all: a manager reaches a project
// container over exactly this connection, so declaring the call runtime-only in
// #139 left its one caller with nowhere to go.
func (c *Client) GetHistoryFloor(ctx context.Context, r *api.SessionRef, opts ...grpc.CallOption) (*api.HistoryFloorReply, error) {
	v, err := c.sessions.GetHistoryFloor(ctx, resource.SessionHistoryFloorRequest_builder{
		Ref: sr(r.Id),
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.HistoryFloorReply{Through: v.GetThrough()}, nil
}

func resourceEngineSpec(v *api.EngineSpec) *resource.EngineSpec {
	if v == nil {
		return nil
	}
	mode := enginemode.Of(v.Mode)
	return resource.EngineSpec_builder{Mode: &mode, Image: &v.Image, Override: v.Override}.Build()
}

func engineReply(v *resource.EngineReply, err error) (*api.EngineReply, error) {
	if err != nil {
		return nil, err
	}
	return &api.EngineReply{Status: v.GetStatus()}, nil
}
