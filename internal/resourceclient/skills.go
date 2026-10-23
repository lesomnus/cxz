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

func (c *Client) GetSkills(ctx context.Context, r *api.SkillsInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	in, err := c.projectHandle(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.skills.List(ctx, resource.SkillListRequest_builder{In: &in}.Build(), opts...))
}

func (c *Client) AddSkill(ctx context.Context, r *api.SkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	in, err := c.projectHandle(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.skills.Add(ctx, resource.SkillAddRequest_builder{
		Name: &r.Name, In: &in,
	}.Build(), opts...))
}

func (c *Client) RemoveSkill(ctx context.Context, r *api.SkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	in, err := c.projectHandle(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.skills.Remove(ctx, resource.SkillRemoveRequest_builder{
		Name: &r.Name, In: &in,
	}.Build(), opts...))
}

func (c *Client) SetSkillDefault(ctx context.Context, r *api.SkillDefaultInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	return skillsReply(c.skills.SetDefault(ctx, resource.SkillDefaultRequest_builder{
		Name: &r.Name, Enabled: &r.Enabled,
	}.Build(), opts...))
}

func (c *Client) SetProjectSkill(ctx context.Context, r *api.ProjectSkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	in, err := c.projectHandle(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.skills.Enable(ctx, resource.SkillEnableRequest_builder{
		Name: &r.Name, In: &in, Enabled: &r.Enabled,
	}.Build(), opts...))
}

func (c *Client) ClearProjectSkill(ctx context.Context, r *api.ClearProjectSkillInput, opts ...grpc.CallOption) (*api.SkillsReply, error) {
	in, err := c.projectHandle(ctx, r.Project, opts...)
	if err != nil {
		return nil, err
	}
	return skillsReply(c.skills.Clear(ctx, resource.SkillClearRequest_builder{
		Name: &r.Name, In: &in,
	}.Build(), opts...))
}

// SyncSkills is the manager's call to a project container, over the runtime
// API. This client talks to a manager's resource API, and a manager does not
// deliver to itself -- so there is nothing here to deliver to, and the resource
// API no longer carries the call at all.
// See docs/plans/service-boundaries.md.
func (c *Client) SyncSkills(context.Context, *api.SyncSkillsInput, ...grpc.CallOption) (*api.Receipt, error) {
	return nil, status.Error(codes.Unimplemented, "skills are delivered by a manager to a project over the runtime API")
}

func (c *Client) RenderDevcontainer(ctx context.Context, r *api.RenderDevcontainerInput, opts ...grpc.CallOption) (*api.RenderDevcontainerReply, error) {
	v, err := c.installation.RenderDevcontainer(ctx, resource.RenderDevcontainerRequest_builder{
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

// projectRef turns the handle a caller used -- an id, a path, an alias or a
// display name -- into a ref. A ref is a key to a row and a handle is not, so
// the translation happens here, against the same list the CLI prints, rather
// than being pushed onto the server as another string to guess at. An empty
// handle leaves the ref unset: the installation's own scope is a scope, not a
// project named "".
func (c *Client) projectRef(ctx context.Context, handle string, opts ...grpc.CallOption) (*resource.ProjectRef, error) {
	id, err := c.projectHandle(ctx, handle, opts...)
	if err != nil || id == "" {
		return nil, err
	}
	return pr(id), nil
}

// projectHandle resolves what a person typed to the project's runtime id, which
// is what the services scoped by a project take. Resolving here is not new: the
// ref this used to build was resolved the same way, and the server turned it
// straight back into this id. An empty handle stays empty -- it is the
// installation's own scope and not a missing argument.
func (c *Client) projectHandle(ctx context.Context, handle string, opts ...grpc.CallOption) (string, error) {
	if handle == "" {
		return "", nil
	}
	ps, err := c.Projects(ctx, &api.Empty{}, opts...)
	if err != nil {
		return "", err
	}
	p, err := projectref.Resolve(ps.Projects, handle)
	if err != nil {
		return "", err
	}
	return p.Id, nil
}

func skillsReply(v *resource.SkillList, err error) (*api.SkillsReply, error) {
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
