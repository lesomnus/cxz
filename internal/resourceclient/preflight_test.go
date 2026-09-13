package resourceclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type preflightProjects struct {
	resource.ProjectServiceClient
	items  []*resource.Project
	writes int
}

func (p *preflightProjects) List(context.Context, *resource.ProjectListRequest, ...grpc.CallOption) (*resource.ProjectListResponse, error) {
	return resource.ProjectListResponse_builder{Items: p.items}.Build(), nil
}
func (p *preflightProjects) Add(context.Context, *resource.ProjectAddRequest, ...grpc.CallOption) (*resource.Project, error) {
	p.writes++
	return p.items[0], nil
}
func (p *preflightProjects) Get(context.Context, *resource.ProjectGetRequest, ...grpc.CallOption) (*resource.Project, error) {
	return p.items[0], nil
}
func (p *preflightProjects) Up(context.Context, *resource.ProjectUpRequest, ...grpc.CallOption) (*resource.Project, error) {
	p.writes++
	return p.items[0], nil
}
func (p *preflightProjects) Recreate(context.Context, *resource.ProjectRecreateRequest, ...grpc.CallOption) (*resource.Project, error) {
	p.writes++
	return p.items[0], nil
}

type preflightAccounts struct{ resource.AccountServiceClient }

func (preflightAccounts) Get(_ context.Context, r *resource.AccountGetRequest, _ ...grpc.CallOption) (*resource.Account, error) {
	alias := r.GetRef().GetAlias()
	if alias == "missing" {
		return nil, status.Error(codes.NotFound, "account not found")
	}
	backend := "project-local-oauth"
	if alias == "unsupported" {
		backend = "unknown"
	}
	return resource.Account_builder{Alias: alias, Agent: "codex", AuthBackend: backend}.Build(), nil
}

type preflightSessions struct {
	resource.SessionServiceClient
	items []*resource.Session
	err   error
}

func (s preflightSessions) List(context.Context, *resource.SessionListRequest, ...grpc.CallOption) (*resource.SessionListResponse, error) {
	return resource.SessionListResponse_builder{Items: s.items}.Build(), s.err
}
func fixtureSession(p *resource.Project, state string) *resource.Session {
	return resource.Session_builder{RuntimeId: "thread", Agent: "codex", Model: "original", ClientId: "creation", Project: p, Account: resource.Account_builder{Alias: "work", Agent: "codex", AuthBackend: "project-local-oauth"}.Build(), Status: resource.SessionStatus_builder{State: state}.Build()}.Build()
}

func TestConcurrentSessionSelection(t *testing.T) {
	list := []*api.Session{{Id: "one", ProjectId: "p", Agent: "claude", Account: "work", State: "working", CreateId: "first"}, {Id: "two", ProjectId: "p", Agent: "codex", Account: "personal", State: "idle", CreateId: "second"}}
	for _, tc := range []struct {
		r  *api.ProjectRequest
		id string
	}{
		{&api.ProjectRequest{NewSession: true, Agent: "claude", Account: "work", ClientId: "third"}, ""},
		{&api.ProjectRequest{NewSession: true, Agent: "claude", Account: "work", ClientId: "first"}, "one"},
		{&api.ProjectRequest{Agent: "codex", Account: "personal"}, "two"},
		{&api.ProjectRequest{Agent: "claude", Account: "personal"}, ""},
	} {
		_, s, err := chooseSession(list, "p", tc.r, false)
		if err != nil {
			t.Fatal(err)
		}
		id := ""
		if s != nil {
			id = s.Id
		}
		if id != tc.id {
			t.Fatalf("want %q got %q", tc.id, id)
		}
	}
}
func TestOpenFailsBeforeAnyProjectMutation(t *testing.T) {
	cases := []struct {
		name                  string
		r                     *api.ProjectRequest
		state, account, model string
		unregistered          bool
		want                  string
	}{
		{name: "first up", want: "--account", unregistered: true},
		{name: "registered empty project", want: "--account"},
		{name: "new", r: &api.ProjectRequest{NewSession: true}, want: "--account"},
		{name: "recreate", r: &api.ProjectRequest{Recreate: true, Confirmed: true}, want: "--account"},
		{name: "unknown account", account: "missing", want: "account not found"},
		{name: "unsupported backend", account: "unsupported", want: "auth backend"},
		{name: "agent mismatch", r: &api.ProjectRequest{Agent: "claude"}, account: "work", want: "agent does not match"},
		{name: "invalid model", account: "work", model: "bad model", want: "model must"},
		{name: "immutable model", state: "idle", model: "changed", want: "model is immutable"},
		{name: "immutable model before recreate", state: "idle", r: &api.ProjectRequest{Recreate: true, Confirmed: true}, model: "changed", want: "model is immutable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := resource.Project_builder{RuntimeId: "project", Workspace: "/project", Alias: "alias"}.Build()
			projects := &preflightProjects{items: []*resource.Project{p}}
			if tc.unregistered {
				projects.items = nil
			}
			sessions := preflightSessions{}
			if tc.state != "" {
				sessions.items = []*resource.Session{fixtureSession(p, tc.state)}
			}
			c := &Client{projects: projects, sessions: sessions, Accounts: preflightAccounts{}}
			r := tc.r
			if r == nil {
				r = &api.ProjectRequest{}
			}
			r.Workspace = "/project"
			r.Account = tc.account
			r.Model = tc.model
			_, err := c.Open(context.Background(), r)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
			if projects.writes != 0 {
				t.Fatal("project mutated before validation")
			}
		})
	}
}
func TestExistingSessionAndPrepareOnlyNeedNoAccountFlag(t *testing.T) {
	p := resource.Project_builder{RuntimeId: "project", Workspace: "/project", Alias: "alias"}.Build()
	for _, state := range []string{"idle", "working", "waiting_input", "stopped", "interrupted", "failed"} {
		projects := &preflightProjects{items: []*resource.Project{p}}
		c := &Client{projects: projects, sessions: preflightSessions{items: []*resource.Session{fixtureSession(p, state)}}, Accounts: preflightAccounts{}}
		r := &api.ProjectRequest{Workspace: "alias"}
		if err := c.CheckOpen(context.Background(), r); err != nil {
			t.Fatal(state, err)
		}
		if r.Account != "work" || r.Agent != "codex" || projects.writes != 0 {
			t.Fatal("existing identity not preserved", r)
		}
		if state == "idle" {
			s, err := c.Open(context.Background(), r)
			if err != nil || s.Id != "thread" {
				t.Fatal("live attach", err)
			}
		}
	}
	c := &Client{} // PrepareOnly without an account requires no session lookup.
	if err := c.CheckOpen(context.Background(), &api.ProjectRequest{PrepareOnly: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Create(context.Background(), &api.CreateRequest{}); !errors.Is(err, ErrAccountRequired) {
		t.Fatal(err)
	}
}
func TestPreflightFailsClosedOnSessionLookupFailure(t *testing.T) {
	projects := &preflightProjects{items: []*resource.Project{resource.Project_builder{RuntimeId: "project", Workspace: "/project"}.Build()}}
	c := &Client{projects: projects, sessions: preflightSessions{err: status.Error(codes.Unavailable, "offline")}}
	_, err := c.Open(context.Background(), &api.ProjectRequest{Workspace: "/project"})
	if status.Code(err) != codes.Unavailable || projects.writes != 0 {
		t.Fatal("lookup failure treated as no session", err)
	}
}
