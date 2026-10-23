package resourceclient

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectref"
	"github.com/lesomnus/cxz/internal/projectstate"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func project(p *resource.Project) *api.Project {
	st := p.GetStatus()
	return &api.Project{Id: p.GetRuntimeId(), Alias: p.GetAlias(), Workspace: p.GetWorkspace(), Name: p.GetName(), Config: p.GetConfig(), State: projectstate.Word(st.GetContainerState()), ContainerId: st.GetContainerId(), RemoteUser: st.GetRemoteUser(), RemoteWorkspace: st.GetRemoteWorkspace(), ProvisionState: st.GetProvisionState(), ProvisionStep: st.GetProvisionStep(), ProvisionAttempt: st.GetProvisionAttempt(), Error: st.GetError()}
}
func (c *Client) Projects(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.ProjectList, error) {
	out, err := c.registeredProjects(ctx, opts...)
	if err != nil {
		return nil, err
	}
	foreign, err := c.installation.ForeignContainers(ctx, &resource.ForeignContainersRequest{}, opts...)
	if err != nil && status.Code(err) != codes.Unimplemented {
		return nil, err
	}
	if foreign != nil {
		for _, p := range foreign.GetItems() {
			out.Projects = append(out.Projects, &api.Project{Workspace: p.GetWorkspace(), Name: p.GetName(), ContainerId: p.GetContainerId(), State: "foreign"})
		}
	}
	return out, nil
}

// RegisteredProjects avoids external Docker discovery in the TUI refresh path.
func (c *Client) RegisteredProjects(ctx context.Context) (*api.ProjectList, error) {
	return c.registeredProjects(ctx)
}
func (c *Client) registeredProjects(ctx context.Context, opts ...grpc.CallOption) (*api.ProjectList, error) {
	out := &api.ProjectList{}
	after := ""
	for {
		page, err := c.projects.List(ctx, resource.ProjectListRequest_builder{Size: 200, After: after}.Build(), opts...)
		if err != nil {
			return nil, err
		}
		for _, p := range page.GetItems() {
			out.Projects = append(out.Projects, project(p))
		}
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	return out, nil
}

// Add resolves the manager's workspace path, runtime ID or unambiguous name.
// Session selection is a client workflow; Project.Up never creates a session.
func (c *Client) Open(ctx context.Context, r *api.ProjectRequest, opts ...grpc.CallOption) (*api.Session, error) {
	if err := c.CheckOpen(ctx, r, opts...); err != nil {
		return nil, err
	}
	p, err := c.openProject(ctx, r, opts...)
	if err != nil {
		return nil, err
	}
	ref := pr(p.GetRuntimeId())
	if p, err = c.start(ctx, ref, r, opts...); err != nil {
		return nil, err
	}
	if r.PrepareOnly {
		return &api.Session{ProjectId: p.GetRuntimeId(), Workspace: p.GetStatus().GetRemoteWorkspace()}, nil
	}
	list, err := c.List(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil, err
	}
	kind, chosen, err := chooseSession(list.Sessions, p.GetRuntimeId(), r, false)
	if err != nil {
		return nil, err
	}
	if chosen == nil {
		if r.Account == "" {
			return nil, ErrAccountRequired
		}
		b, err := c.Bind(ctx, p.GetRuntimeId(), r.Account)
		if err != nil {
			return nil, err
		}
		s, err := c.sessions.Add(ctx, resource.SessionAddRequest_builder{Project: ref, Agent: kind, Model: r.Model, ClientId: r.ClientId, Account: ar(r.Account), AuthBinding: br(b.GetBindingId())}.Build(), opts...)
		if err != nil {
			if !r.NewSession {
				if attached := c.concurrentOpen(ctx, p.GetRuntimeId(), kind, r.Model, r.Account, opts...); attached != nil {
					return attached, nil
				}
			}
			return nil, err
		}
		return c.view(ctx, s, opts...)
	}
	if r.Model != "" && r.Model != chosen.Model {
		return nil, fmt.Errorf("existing session model is immutable; stop it and create a new session")
	}
	if chosen.State == "interrupted" || chosen.State == "stopped" || chosen.State == "failed" {
		resumed, err := c.Resume(ctx, &api.Control{SessionId: chosen.Id, RunId: chosen.RunId, ClientId: r.ClientId}, opts...)
		if err != nil && !r.NewSession {
			if attached := c.concurrentOpen(ctx, p.GetRuntimeId(), kind, r.Model, chosen.Account, opts...); attached != nil && attached.Id == chosen.Id {
				return attached, nil
			}
		}
		return resumed, err
	}
	return chosen, nil
}

// Another up may finish between List and Add/Resume. Re-read, never resend an
// effect or relax explicit `new` conflicts. Only an already live match attaches.
func (c *Client) concurrentOpen(ctx context.Context, project, agent, model, account string, opts ...grpc.CallOption) *api.Session {
	list, err := c.List(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil
	}
	for _, s := range list.Sessions {
		if s.ProjectId == project && s.Agent == agent && s.Account == account && (model == "" || s.Model == model) && (s.State == "idle" || s.State == "working" || s.State == "waiting_input" || s.State == "starting") {
			return s
		}
	}
	return nil
}
func (c *Client) RemoveContainer(ctx context.Context, r *api.ProjectRequest, opts ...grpc.CallOption) (*api.Receipt, error) {
	// Resolve without registering a new resource as a side effect of shutdown.
	ps, err := c.Projects(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil, err
	}
	p, err := projectref.Resolve(ps.Projects, r.Workspace)
	if err != nil {
		return nil, err
	}
	id := p.Id
	_, err = c.projects.Remove(ctx, resource.ProjectControl_builder{Ref: pr(id), ClientId: &r.ClientId}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: "stopped"}, nil
}

// StartContainer makes the project run and nothing else. `Open` is the
// composition on top of it: start, then decide about a session.
func (c *Client) StartContainer(ctx context.Context, r *api.ProjectRequest, opts ...grpc.CallOption) (*api.Receipt, error) {
	ps, err := c.Projects(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil, err
	}
	p, err := projectref.Resolve(ps.Projects, r.Workspace)
	if err != nil {
		return nil, err
	}
	if _, err = c.start(ctx, pr(p.Id), r, opts...); err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: "started"}, nil
}

// start makes a project run, and replaces its container first when that is
// what was asked for. Replacing is two calls, in this order, and this is the
// one place that composes them: the server has no operation that removes a
// container as a step towards making one, because deciding to lose a container
// is not a step -- it is the whole question. A recreate without confirmation
// never gets this far; the CLI and the UI ask first.
func (c *Client) start(ctx context.Context, ref *resource.ProjectRef, r *api.ProjectRequest, opts ...grpc.CallOption) (*resource.Project, error) {
	if r.Recreate {
		if !r.Confirmed {
			return nil, fmt.Errorf("recreate requires confirmation: writable layer lost and editors disconnect")
		}
		if _, err := c.projects.Remove(ctx, resource.ProjectControl_builder{Ref: ref, ClientId: &r.ClientId}.Build(), opts...); err != nil {
			return nil, err
		}
	}
	return c.projects.Start(ctx, resource.ProjectStartRequest_builder{Ref: ref, ClientId: &r.ClientId, Agent: &r.Agent, TrustConfig: &r.TrustConfig, Config: &r.Config}.Build(), opts...)
}

// StopContainer and RemoveContainer resolve the same way, without registering a
// resource as a side effect of stopping or removing one.
func (c *Client) StopContainer(ctx context.Context, r *api.ProjectRequest, opts ...grpc.CallOption) (*api.Receipt, error) {
	return c.container(ctx, r, c.projects.Stop, "stopped", opts...)
}

func (c *Client) container(ctx context.Context, r *api.ProjectRequest, call func(context.Context, *resource.ProjectControl, ...grpc.CallOption) (*resource.Project, error), status string, opts ...grpc.CallOption) (*api.Receipt, error) {
	ps, err := c.Projects(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil, err
	}
	p, err := projectref.Resolve(ps.Projects, r.Workspace)
	if err != nil {
		return nil, err
	}
	if _, err = call(ctx, resource.ProjectControl_builder{Ref: pr(p.Id), ClientId: &r.ClientId}.Build(), opts...); err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: status}, nil
}

// EraseProject deletes the project's data. There is no preview step: the ref is
// a UUID, so there is nothing to resolve wrongly, which is what the preview was
// guarding against. Confirmation belongs to whoever is asking a person.
func (c *Client) EraseProject(ctx context.Context, target string) error {
	p, err := c.ResolveProject(ctx, target)
	if err != nil {
		return err
	}
	_, err = c.projects.Erase(ctx, pr(p.Id))
	return err
}
