package resourceclient

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectref"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) ResolveProject(ctx context.Context, handle string, opts ...grpc.CallOption) (*api.Project, error) {
	var items []*api.Project
	after := ""
	for {
		page, err := c.projects.List(ctx, resource.ProjectListRequest_builder{Filters: []*resource.ProjectFilter{resource.ProjectFilter_builder{Listed: ptr(true)}.Build()}, Size: 200, After: after}.Build(), opts...)
		if err != nil {
			return nil, err
		}
		for _, p := range page.GetItems() {
			items = append(items, project(p))
		}
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	return projectref.Resolve(items, handle)
}
func (c *Client) AddProject(ctx context.Context, path, name, alias, config string) (*api.Project, error) {
	p, err := c.projects.Add(ctx, resource.ProjectAddRequest_builder{Workspace: path, Name: name, Alias: alias, Config: config}.Build())
	if err != nil {
		return nil, err
	}
	return project(p), nil
}
func (c *Client) SetProject(ctx context.Context, handle string, name, alias *string) (*api.Project, error) {
	p, err := c.ResolveProject(ctx, handle)
	if err != nil {
		return nil, err
	}
	current, err := c.projects.Get(ctx, resource.ProjectGetRequest_builder{Ref: pr(p.Id), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	updated, err := c.projects.Patch(ctx, resource.ProjectPatchRequest_builder{Ref: pr(p.Id), Name: name, Alias: alias, DateUpdated: current.GetDateUpdated()}.Build())
	if err != nil {
		return nil, err
	}
	return project(updated), nil
}
func (c *Client) openProject(ctx context.Context, r *api.ProjectRequest, opts ...grpc.CallOption) (*resource.Project, error) {
	p, err := c.ResolveProject(ctx, r.Workspace, opts...)
	if status.Code(err) == codes.NotFound {
		return c.projects.Add(ctx, resource.ProjectAddRequest_builder{Workspace: r.Workspace, Config: r.Config, Name: r.Name, Alias: r.Alias}.Build(), opts...)
	}
	if err != nil {
		return nil, err
	}
	if r.Name != "" || r.Alias != "" {
		var name, alias *string
		if r.Name != "" {
			name = &r.Name
		}
		if r.Alias != "" {
			alias = &r.Alias
		}
		if _, err = c.SetProject(ctx, p.Id, name, alias); err != nil {
			return nil, err
		}
	}
	return c.projects.Get(ctx, resource.ProjectGetRequest_builder{Ref: pr(p.Id), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build(), opts...)
}
