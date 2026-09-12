// Package resourceclient adapts the existing TUI view models to the payday
// resource API. Runtime view models are not a public wire API.
package resourceclient

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

type Client struct {
	projects resource.ProjectServiceClient
	sessions resource.SessionServiceClient
	Accounts resource.AccountServiceClient
}

func New(conn grpc.ClientConnInterface) *Client {
	return &Client{resource.NewProjectServiceClient(conn), resource.NewSessionServiceClient(conn), resource.NewAccountServiceClient(conn)}
}

var _ api.SessionsClient = (*Client)(nil)

func ptr[T any](v T) *T                 { return &v }
func sr(id string) *resource.SessionRef { return resource.SessionRef_builder{RuntimeId: &id}.Build() }
func pr(id string) *resource.ProjectRef { return resource.ProjectRef_builder{RuntimeId: &id}.Build() }
func (c *Client) view(ctx context.Context, s *resource.Session, opts ...grpc.CallOption) (*api.Session, error) {
	p := s.GetProject()
	if p != nil && p.GetRuntimeId() == "" {
		var err error
		p, err = c.projects.Get(ctx, resource.ProjectGetRequest_builder{Ref: resource.ProjectRef_builder{Id: p.GetId()}.Build(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build(), opts...)
		if err != nil {
			return nil, err
		}
	}
	st := s.GetStatus()
	v := &api.Session{Id: s.GetRuntimeId(), Title: s.GetName(), Agent: s.GetAgent(), Model: s.GetModel(), CreateId: s.GetClientId(), ProjectId: p.GetRuntimeId(), Workspace: p.GetWorkspace(), State: st.GetState(), RunId: st.GetRunId(), VendorId: st.GetVendorId(), LastSeq: st.GetLastSeq()}
	v.ProjectName = p.GetName()
	v.ProjectAlias = p.GetAlias()
	if a := s.GetAccount(); a != nil {
		if a.GetAlias() == "" {
			var err error
			a, err = c.Accounts.Get(ctx, resource.AccountGetRequest_builder{Ref: resource.AccountRef_builder{Id: a.GetId()}.Build(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build(), opts...)
			if err != nil {
				return nil, err
			}
		}
		v.Account = a.GetAlias()
	}
	if s.GetDateCreated() != nil {
		v.CreatedAt = s.GetDateCreated().AsTime().UnixMilli()
	}
	for _, e := range st.GetPending() {
		v.Pending = append(v.Pending, event(v.Id, e))
	}
	return v, nil
}
func (c *Client) Create(ctx context.Context, r *api.CreateRequest, opts ...grpc.CallOption) (*api.Session, error) {
	p, err := c.projects.Add(ctx, resource.ProjectAddRequest_builder{Workspace: r.Workspace}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	if p.GetStatus().GetState() != "running" {
		if _, err = c.projects.Up(ctx, resource.ProjectUpRequest_builder{Ref: pr(p.GetRuntimeId()), ClientId: &r.ClientId, Agent: &r.Agent}.Build(), opts...); err != nil {
			return nil, err
		}
	}
	s, err := c.sessions.Add(ctx, resource.SessionAddRequest_builder{Project: pr(p.GetRuntimeId()), Name: r.Title, ClientId: r.ClientId, Agent: r.Agent, Model: r.Model, Account: ar(r.Account)}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return c.view(ctx, s, opts...)
}
func (c *Client) Get(ctx context.Context, r *api.SessionRef, opts ...grpc.CallOption) (*api.Session, error) {
	s, err := c.sessions.Get(ctx, resource.SessionGetRequest_builder{Ref: sr(r.Id), Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return c.view(ctx, s, opts...)
}
func (c *Client) List(ctx context.Context, _ *api.Empty, opts ...grpc.CallOption) (*api.SessionList, error) {
	out := &api.SessionList{}
	after := ""
	for {
		page, err := c.sessions.List(ctx, resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Listed: ptr(true)}.Build()}, Size: 200, After: after}.Build(), opts...)
		if err != nil {
			return nil, err
		}
		for _, s := range page.GetItems() {
			v, err := c.view(ctx, s, opts...)
			if err != nil {
				return nil, err
			}
			out.Sessions = append(out.Sessions, v)
		}
		after = page.GetNext()
		if after == "" {
			return out, nil
		}
	}
}
func control(r *api.Control) *resource.SessionControl {
	return resource.SessionControl_builder{Ref: sr(r.SessionId), RunId: &r.RunId, ClientId: &r.ClientId}.Build()
}
func receipt(v *resource.SessionReceipt, err error) (*api.Receipt, error) {
	if err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: v.GetClientId(), Status: v.GetStatus()}, nil
}
func (c *Client) Resume(ctx context.Context, r *api.Control, opts ...grpc.CallOption) (*api.Session, error) {
	s, err := c.sessions.Resume(ctx, control(r), opts...)
	if err != nil {
		return nil, err
	}
	return c.view(ctx, s, opts...)
}
func (c *Client) Stop(ctx context.Context, r *api.Control, opts ...grpc.CallOption) (*api.Receipt, error) {
	_, err := c.sessions.Stop(ctx, control(r), opts...)
	if err != nil {
		return nil, err
	}
	return &api.Receipt{ClientId: r.ClientId, Status: "accepted"}, nil
}
func (c *Client) Interrupt(ctx context.Context, r *api.Control, opts ...grpc.CallOption) (*api.Receipt, error) {
	return receipt(c.sessions.Interrupt(ctx, control(r), opts...))
}
func (c *Client) Send(ctx context.Context, r *api.Input, opts ...grpc.CallOption) (*api.Receipt, error) {
	return receipt(c.sessions.Send(ctx, resource.SessionSendRequest_builder{Ref: sr(r.SessionId), RunId: &r.RunId, ClientId: &r.ClientId, Text: &r.Text}.Build(), opts...))
}
func (c *Client) Reply(ctx context.Context, r *api.Answer, opts ...grpc.CallOption) (*api.Receipt, error) {
	return receipt(c.sessions.Reply(ctx, resource.SessionReplyRequest_builder{Ref: sr(r.SessionId), RunId: &r.RunId, ClientId: &r.ClientId, RequestId: &r.RequestId, Allow: &r.Allow, AnswersJson: &r.AnswersJson}.Build(), opts...))
}
func event(id string, e *resource.SessionEvent) *api.Event {
	return &api.Event{SessionId: id, RunId: e.GetRunId(), Seq: e.GetSeq(), TimeMs: e.GetTimeMs(), Kind: e.GetKind(), Text: e.GetText(), RequestId: e.GetRequestId(), Payload: e.GetPayload()}
}

type stream struct {
	grpc.ServerStreamingClient[resource.SessionEvent]
	id string
}

func (s stream) Recv() (*api.Event, error) {
	e, err := s.ServerStreamingClient.Recv()
	if err != nil {
		return nil, err
	}
	return event(s.id, e), nil
}
func (c *Client) Watch(ctx context.Context, r *api.WatchRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.Event], error) {
	s, err := c.sessions.Events(ctx, resource.SessionEventsRequest_builder{Ref: sr(r.SessionId), AfterSeq: &r.AfterSeq}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return stream{s, r.SessionId}, nil
}
func (c *Client) History(ctx context.Context, r *api.WatchRequest, opts ...grpc.CallOption) (*api.EventBatch, error) {
	b, err := c.sessions.History(ctx, resource.SessionEventsRequest_builder{Ref: sr(r.SessionId), AfterSeq: &r.AfterSeq}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	v := &api.EventBatch{}
	for _, e := range b.GetEvents() {
		v.Events = append(v.Events, event(r.SessionId, e))
	}
	return v, nil
}
