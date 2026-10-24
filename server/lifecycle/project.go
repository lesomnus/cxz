package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectid"
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
	if r.HasId() || r.HasStatus() || r.HasDateCreated() {
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
		if owner != nil && owner.GetWorkspace() != path && projectid.String(owner.GetId()) != r.GetWorkspace() {
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
		a, err := s.alias(ctx, projectid.String(p.GetId()), p.GetName(), r.GetAlias())
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

// Start makes the project run: it builds and creates the container when there
// is none, and starts the one there is otherwise. It does not replace one --
// losing a container is `Remove`, asked for on purpose -- and it does not apply
// a changed configuration, because starting cannot: it says so instead.
func (s ProjectServer) Start(ctx context.Context, r *resource.ProjectStartRequest) (*resource.Project, error) {
	return s.start(ctx, r.GetRef(), r.GetClientId(), r.GetAgent(), r.GetConfig(), r.GetTrustConfig())
}

func (s ProjectServer) start(ctx context.Context, ref *resource.ProjectRef, clientID, agent, config string, trust bool) (*resource.Project, error) {
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
	// Making the project run is all this asks for; sessions are the caller's.
	_, err = s.shared.runtime.StartContainer(ctx, &api.ProjectRequest{Workspace: projectid.String(p.GetId()), Config: config, Agent: agent, ClientId: clientID, TrustConfig: trust})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
}

// Stop and Remove are the container's lifecycle without the resource's: the
// project, its sessions and their conversations are untouched, and neither
// touches an agent -- that is Session.Resume.
func (s ProjectServer) Stop(ctx context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	return s.container(ctx, r, s.shared.runtime.StopContainer)
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
	if _, err = call(ctx, &api.ProjectRequest{Workspace: projectid.String(p.GetId()), ClientId: r.GetClientId()}); err != nil {
		return nil, err
	}
	// The answer is observed, not guessed: the read that follows looks at the
	// container again rather than reporting what the operation intended.
	return s.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
}

func (s ProjectServer) Remove(ctx context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return nil, err
	}
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if _, err = s.shared.runtime.RemoveContainer(ctx, &api.ProjectRequest{Workspace: projectid.String(p.GetId()), ClientId: r.GetClientId()}); err != nil {
		return nil, err
	}
	return s.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
}
