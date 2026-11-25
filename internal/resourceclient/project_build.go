package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectref"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) BuildProject(ctx context.Context, r *api.BuildProjectInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	ps, err := c.Projects(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil, err
	}
	p, err := projectref.Resolve(ps.Projects, r.Workspace)
	if err != nil {
		return nil, err
	}
	if _, err = c.projects.Build(ctx, resource.ProjectBuildRequest_builder{Ref: pr(p.Id), ClientId: &r.ClientId, Dockerfile: &r.Dockerfile, Context: &r.Context}.Build(), opts...); err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: "built"}, nil
}
