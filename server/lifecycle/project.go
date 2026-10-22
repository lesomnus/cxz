package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/slug"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"path/filepath"
)

type ProjectServer struct {
	Layer
	resource.ProjectServiceServer
}

func (s ProjectServer) Add(ctx context.Context, r *resource.ProjectAddRequest) (*resource.Project, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return nil, err
	}
	if r.HasId() || r.GetRuntimeId() != "" || r.HasStatus() || r.HasDateCreated() {
		return nil, closed()
	}
	if r.GetWorkspace() == "" {
		return nil, status.Error(codes.InvalidArgument, "workspace required")
	}
	if r.GetName() != "" {
		if err := validName(r.GetName()); err != nil {
			return nil, err
		}
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	// Reject malformed/occupied explicit aliases before registering runtime intent.
	if r.GetAlias() != "" {
		a, err := slug.ParseAlias(r.GetAlias())
		if err != nil {
			return nil, err
		}
		owner, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: resource.ProjectRef_builder{Alias: &a}.Build(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil && status.Code(err) != codes.NotFound {
			return nil, err
		}
		path := r.GetWorkspace()
		if absolute, e := filepath.Abs(path); e == nil {
			if canonical, e := filepath.EvalSymlinks(absolute); e == nil {
				path = canonical
			}
		}
		if owner != nil && owner.GetWorkspace() != path && owner.GetRuntimeId() != r.GetWorkspace() {
			return nil, status.Error(codes.AlreadyExists, "project alias is already in use")
		}
	}
	p, err := s.shared.runtime.RegisterProject(ctx, r.GetWorkspace(), r.GetConfig())
	if err != nil {
		return nil, err
	}
	if r.GetName() != "" {
		p.Name = r.GetName()
	}
	alias, err := s.alias(ctx, p.Id, p.Name, r.GetAlias())
	if err != nil {
		return nil, err
	}
	p.Alias = alias
	v, err := s.saveProject(ctx, p)
	if err != nil {
		return nil, err
	}
	if r.GetName() != "" || r.GetDesc() != "" || r.GetAlias() != "" {
		patch := resource.ProjectPatchRequest_builder{Ref: projectRef(p.Id), DateUpdatedForce: ptr(true)}.Build()
		if r.GetName() != "" {
			patch.SetName(r.GetName())
		}
		if r.GetDesc() != "" {
			patch.SetDesc(r.GetDesc())
		}
		if r.GetAlias() != "" {
			patch.SetAlias(alias)
		}
		return s.Next().Project().Patch(ctx, patch)
	}
	return v, nil
}
func (s ProjectServer) Get(ctx context.Context, r *resource.ProjectGetRequest) (*resource.Project, error) {
	if err := s.ensureSnapshot(ctx); err != nil {
		return nil, err
	}
	if err := s.observeProjects(ctx); err != nil {
		return nil, err
	}
	// A project the store answers with is a project that exists: an erased one
	// is already NotFound, since every read narrows on `date_erased`.
	return s.ProjectServiceServer.Get(ctx, r)
}
// List answers with every registered project. There is no hidden half any
// more: a project is registered or erased, and an erased one is gone.
func (s ProjectServer) List(ctx context.Context, r *resource.ProjectListRequest) (*resource.ProjectListResponse, error) {
	if err := s.ensureSnapshot(ctx); err != nil {
		return nil, err
	}
	if err := s.observeProjects(ctx); err != nil {
		return nil, err
	}
	return s.ProjectServiceServer.List(ctx, r)
}
func (s ProjectServer) Watch(r *resource.ProjectWatchRequest, stream grpc.ServerStreamingServer[resource.ProjectWatchResponse]) error {
	s.shared.watchers.Add(1)
	defer s.shared.watchers.Add(-1)
	if err := s.ensureSnapshot(stream.Context()); err != nil {
		return err
	}
	if err := s.observeProjects(stream.Context()); err != nil {
		return err
	}
	return s.watchInventory(r, stream)
}
func (s ProjectServer) Patch(ctx context.Context, r *resource.ProjectPatchRequest) (*resource.Project, error) {
	if r.HasStatus() || r.HasStatusNull() || r.HasConfig() || r.GetDateUpdatedForce() {
		return nil, closed()
	}
	if r.HasName() {
		if err := validName(r.GetName()); err != nil {
			return nil, err
		}
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	if r.HasAlias() {
		if r.GetAlias() == "" {
			return nil, status.Error(codes.InvalidArgument, "alias cannot be empty")
		}
		p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			return nil, err
		}
		a, err := s.alias(ctx, p.GetRuntimeId(), p.GetName(), r.GetAlias())
		if err != nil {
			return nil, err
		}
		r.SetAlias(a)
	}
	return s.ProjectServiceServer.Patch(ctx, r)
}
func (s ProjectServer) Apply(context.Context, *resource.ProjectApplyRequest) (*resource.Project, error) {
	return nil, closed()
}
// CreateContainer prepares the container and runs it through the Dev Container
// lifecycle. It does not replace one: a project that already has a container
// is answered as it is, and losing a container is `RemoveContainer`, asked for
// on purpose.
func (s ProjectServer) CreateContainer(ctx context.Context, r *resource.ProjectCreateRequest) (*resource.Project, error) {
	return s.create(ctx, r.GetRef(), r.GetClientId(), r.GetAgent(), r.GetConfig(), r.GetTrustConfig())
}

func (s ProjectServer) create(ctx context.Context, ref *resource.ProjectRef, clientID, agent, config string, trust bool) (*resource.Project, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return nil, err
	}
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if config == "" {
		config = p.GetConfig()
	}
	// Creating a container is all this asks for; sessions are the caller's.
	_, err = s.shared.runtime.CreateContainer(ctx, &api.ProjectRequest{Workspace: p.GetRuntimeId(), Config: config, Agent: agent, ClientId: clientID, TrustConfig: trust})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
}

// StopContainer and StartContainer are the container's lifecycle without the
// resource's: the project, its sessions and their conversations are untouched,
// and neither starts an agent -- that is Session.Resume.
func (s ProjectServer) StopContainer(ctx context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	return s.container(ctx, r, s.shared.runtime.StopContainer)
}

func (s ProjectServer) StartContainer(ctx context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	return s.container(ctx, r, s.shared.runtime.StartContainer)
}

func (s ProjectServer) container(ctx context.Context, r *resource.ProjectControl, call func(context.Context, *api.ProjectRequest) (*api.Receipt, error)) (*resource.Project, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return nil, err
	}
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if _, err = call(ctx, &api.ProjectRequest{Workspace: p.GetRuntimeId(), ClientId: r.GetClientId()}); err != nil {
		return nil, err
	}
	// The answer is observed, not guessed: the read that follows looks at the
	// container again rather than reporting what the operation intended.
	return s.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
}

func (s ProjectServer) RemoveContainer(ctx context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return nil, err
	}
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if _, err = s.shared.runtime.RemoveContainer(ctx, &api.ProjectRequest{Workspace: p.GetRuntimeId(), ClientId: r.GetClientId()}); err != nil {
		return nil, err
	}
	return s.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
}
func (s ProjectServer) InspectForeign(ctx context.Context, _ *resource.InspectForeignRequest) (*resource.InspectForeignResponse, error) {
	ps, err := s.shared.runtime.Projects(ctx, &api.Empty{})
	if err != nil {
		return nil, err
	}
	var items []*resource.ForeignContainer
	for _, p := range ps.Projects {
		if p.State == "foreign" {
			items = append(items, resource.ForeignContainer_builder{ContainerId: &p.ContainerId, Workspace: &p.Workspace, Name: &p.Name}.Build())
		}
	}
	return resource.InspectForeignResponse_builder{Items: items}.Build(), nil
}

func (s ProjectServer) FileMappings(ctx context.Context, r *resource.FileMappingsRequest) (*resource.FileMappingsReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.FileMappings(ctx, &api.FileMappingsInput{Bundle: r.GetBundle()})
	if err != nil {
		return nil, err
	}
	return resource.FileMappingsReply_builder{Status: &out.Status}.Build(), nil
}

func (s ProjectServer) Devcontainer(ctx context.Context, r *resource.DevcontainerRequest) (*resource.DevcontainerReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.Devcontainer(ctx, &api.DevcontainerInput{Spec: r.GetSpec()})
	if err != nil {
		return nil, err
	}
	return resource.DevcontainerReply_builder{Status: &out.Status}.Build(), nil
}

// RenderDevcontainer passes the handle through rather than resolving it. It
// may name a directory inside a workspace, and only the manager knows which
// registered workspace owns that directory -- a ref resolved here would have
// to be the workspace already, which is the thing the caller is asking about.
func (s ProjectServer) RenderDevcontainer(ctx context.Context, r *resource.RenderDevcontainerRequest) (*resource.RenderDevcontainerReply, error) {
	out, err := s.shared.runtime.RenderDevcontainer(ctx, &api.RenderDevcontainerInput{Handle: r.GetHandle()})
	if err != nil {
		return nil, err
	}
	reply := resource.RenderDevcontainerReply_builder{
		Project: &out.Project, Name: &out.Name, Workspace: &out.Workspace, Note: &out.Note,
	}
	for _, f := range out.Files {
		reply.Files = append(reply.Files, resource.RenderedFile_builder{
			Name: &f.Name, Source: &f.Source, Role: &f.Role, Data: f.Data,
		}.Build())
	}
	return reply.Build(), nil
}
