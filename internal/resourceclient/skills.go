package resourceclient

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectref"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

func (c *Client) GetSkills(ctx context.Context, r *api.SkillsInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	ref, err := c.skillRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.projects.GetSkills(ctx, resource.SkillsRequest_builder{Ref: ref}.Build(), opts...))
}

func (c *Client) AddSkill(ctx context.Context, r *api.SkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	ref, err := c.skillRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.projects.AddSkill(ctx, resource.SkillRequest_builder{
		Ref: ref, Name: &r.Name,
	}.Build(), opts...))
}

func (c *Client) RemoveSkill(ctx context.Context, r *api.SkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	ref, err := c.skillRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.projects.RemoveSkill(ctx, resource.SkillRequest_builder{
		Ref: ref, Name: &r.Name,
	}.Build(), opts...))
}

func (c *Client) SetSkillDefault(ctx context.Context, r *api.SkillDefaultInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	return skillsReply(c.projects.SetSkillDefault(ctx, resource.SkillDefaultRequest_builder{
		Name: &r.Name, Enabled: &r.Enabled,
	}.Build(), opts...))
}

func (c *Client) SetProjectSkill(ctx context.Context, r *api.ProjectSkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	ref, err := c.skillRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.projects.SetProjectSkill(ctx, resource.ProjectSkillRequest_builder{
		Ref: ref, Name: &r.Name, Enabled: &r.Enabled,
	}.Build(), opts...))
}

func (c *Client) ClearProjectSkill(ctx context.Context, r *api.ClearProjectSkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	ref, err := c.skillRef(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.projects.ClearProjectSkill(ctx, resource.ClearProjectSkillRequest_builder{
		Ref: ref, Name: &r.Name,
	}.Build(), opts...))
}

func (c *Client) SyncSkills(ctx context.Context, r *api.SyncSkillsInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	out, err := c.projects.SyncSkills(ctx, resource.SyncSkillsRequest_builder{Bundle: r.Bundle}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.Receipt{Status: out.GetStatus()}, nil
}

func (c *Client) RenderDevcontainer(ctx context.Context, r *api.RenderDevcontainerInput, opts ...grpc.CallOption) (*api.RenderDevcontainerReply, error) {
	v, err := c.projects.RenderDevcontainer(ctx, resource.RenderDevcontainerRequest_builder{
		Handle: &r.Handle,
	}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	out := &api.RenderDevcontainerReply{
		Project: v.GetProject(), Name: v.GetName(), Workspace: v.GetWorkspace(), Note: v.GetNote(),
	}
	for _, f := range v.GetFiles() {
		out.Files = append(out.Files, &api.RenderedFile{
			Name: f.GetName(), Source: f.GetSource(), Role: f.GetRole(), Data: f.GetData(),
		})
	}
	return out, nil
}

// skillRef turns the handle a caller used -- an id, a path, an alias or a
// display name -- into a ref. A ref is a key to a row and a handle is not, so
// the translation happens here, against the same list the CLI prints, rather
// than being pushed onto the server as another string to guess at. An empty
// handle leaves the ref unset: the installation's own scope is a scope, not a
// project named "".
func (c *Client) skillRef(ctx context.Context, handle string, opts ...grpc.CallOption) (*resource.ProjectRef, error) {
	if handle == "" {
		return nil, nil
	}
	ps, err := c.Projects(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil, err
	}
	p, err := projectref.Resolve(ps.Projects, handle)
	if err != nil {
		return nil, err
	}
	return pr(p.Id), nil
}

func skillsReply(v *resource.SkillsReply, err error) (*api.SkillsReply, error) {
	if err != nil {
		return nil, err
	}
	out := &api.SkillsReply{Message: v.GetMessage()}
	for _, e := range v.GetEntries() {
		entry := &api.SkillEntry{
			Name: e.GetName(), Description: e.GetDescription(), Effective: e.GetEffective(),
		}
		if e.HasOverride() {
			override := e.GetOverride()
			entry.Override = &override
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}
