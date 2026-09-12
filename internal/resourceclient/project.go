package resourceclient

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectref"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func project(p *resource.Project) *api.Project {
	st := p.GetStatus()
	return &api.Project{Id: p.GetRuntimeId(), Alias: p.GetAlias(), Workspace: p.GetWorkspace(), Name: p.GetName(), Config: p.GetConfig(), State: st.GetState(), ContainerId: st.GetContainerId(), RemoteUser: st.GetRemoteUser(), RemoteWorkspace: st.GetRemoteWorkspace(), ProvisionState: st.GetProvisionState(), ProvisionStep: st.GetProvisionStep(), ProvisionAttempt: st.GetProvisionAttempt(), Error: st.GetError()}
}
func (c *Client) Projects(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.ProjectList, error) {
	out := &api.ProjectList{}
	after := ""
	for {
		page, err := c.projects.List(ctx, resource.ProjectListRequest_builder{Filters: []*resource.ProjectFilter{resource.ProjectFilter_builder{Listed: ptr(true)}.Build()}, Size: 200, After: after}.Build(), opts...)
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
	foreign, err := c.projects.InspectForeign(ctx, &resource.InspectForeignRequest{}, opts...)
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

// Add resolves the manager's workspace path, runtime ID or unambiguous name.
// Session selection is a client workflow; Project.Up never creates a session.
func (c *Client) Open(ctx context.Context, r *api.ProjectRequest, opts ...grpc.CallOption) (*api.Session, error) {
	if r.Account != "" {
		a, err := c.Account(ctx, r.Account)
		if err != nil {
			return nil, err
		}
		if r.Agent != "" && r.Agent != a.GetAgent() {
			return nil, fmt.Errorf("agent does not match account %s", r.Account)
		}
		r.Agent = a.GetAgent()
	}
	p, err := c.openProject(ctx, r, opts...)
	if err != nil {
		return nil, err
	}
	ref := pr(p.GetRuntimeId())
	if r.Recreate {
		p, err = c.projects.Recreate(ctx, resource.ProjectRecreateRequest_builder{Ref: ref, ClientId: &r.ClientId, Agent: &r.Agent, TrustConfig: &r.TrustConfig, Confirmed: &r.Confirmed, Config: &r.Config}.Build(), opts...)
	} else {
		p, err = c.projects.Up(ctx, resource.ProjectUpRequest_builder{Ref: ref, ClientId: &r.ClientId, Agent: &r.Agent, TrustConfig: &r.TrustConfig, Config: &r.Config}.Build(), opts...)
	}
	if err != nil {
		return nil, err
	}
	if r.PrepareOnly {
		return &api.Session{ProjectId: p.GetRuntimeId(), Workspace: p.GetStatus().GetRemoteWorkspace()}, nil
	}
	list, err := c.List(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil, err
	}
	kind := r.Agent
	if kind == "" {
		for _, s := range list.Sessions {
			if s.ProjectId == p.GetRuntimeId() {
				kind = s.Agent
				break
			}
		}
	}
	if kind == "" {
		kind = "claude"
	}
	var chosen *api.Session
	for _, s := range list.Sessions {
		if s.ProjectId != p.GetRuntimeId() {
			continue
		}
		live := s.State == "idle" || s.State == "working" || s.State == "waiting_input" || s.State == "starting"
		if live {
			if r.NewSession || s.Agent != kind || (r.Account != "" && s.Account != r.Account) {
				if r.NewSession && s.Agent == kind && s.Account == r.Account && s.CreateId == r.ClientId {
					chosen = s
					break
				}
				return nil, fmt.Errorf("workspace has an active %s session %s; stop it explicitly before starting another", s.Agent, s.Id)
			}
			chosen = s
			break
		}
		if !r.NewSession && chosen == nil && s.Agent == kind && (r.Account == "" || s.Account == r.Account) {
			chosen = s
		}
	}
	if chosen == nil {
		s, err := c.sessions.Add(ctx, resource.SessionAddRequest_builder{Project: ref, Agent: kind, Model: r.Model, ClientId: r.ClientId, Account: ar(r.Account)}.Build(), opts...)
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
func (c *Client) Down(ctx context.Context, r *api.ProjectRequest, opts ...grpc.CallOption) (*api.Receipt, error) {
	// Resolve without registering a new resource as a side effect of deletion.
	ps, err := c.Projects(ctx, &api.Empty{}, opts...)
	if err != nil {
		return nil, err
	}
	p, err := projectref.Resolve(ps.Projects, r.Workspace)
	if err != nil {
		return nil, err
	}
	id := p.Id
	_, err = c.projects.Down(ctx, resource.ProjectControl_builder{Ref: pr(id), ClientId: &r.ClientId}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: "stopped"}, nil
}
