// Package resourceclient adapts the existing TUI view models to the payday
// resource API. Runtime view models are not a public wire API.
package resourceclient

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
)

type Client struct {
	projects resource.ProjectServiceClient
	sessions resource.SessionServiceClient
	Accounts resource.AccountServiceClient
	Bindings resource.AuthBindingServiceClient
}

func New(conn grpc.ClientConnInterface) *Client {
	return &Client{resource.NewProjectServiceClient(conn), resource.NewSessionServiceClient(conn), resource.NewAccountServiceClient(conn), resource.NewAuthBindingServiceClient(conn)}
}

var _ api.SessionsClient = (*Client)(nil)

func (c *Client) Attach(ctx context.Context, r *api.AttachmentInput, opts ...grpc.CallOption) (*api.Attachment, error) {
	v, err := c.sessions.Attach(ctx, resource.SessionAttachRequest_builder{Ref: sr(r.SessionId), RunId: &r.RunId, Content: r.Content}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.Attachment{Path: v.GetPath()}, nil
}

func ptr[T any](v T) *T { return &v }
func sr(id string) *resource.SessionRef {
	if sessionalias.Valid(id) {
		return resource.SessionRef_builder{Alias: &id}.Build()
	}
	return resource.SessionRef_builder{RuntimeId: &id}.Build()
}
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
	v := &api.Session{Id: s.GetRuntimeId(), Title: s.GetName(), Agent: s.GetAgent(), Model: s.GetModel(), CreateId: s.GetClientId(), ProjectId: p.GetRuntimeId(), Workspace: p.GetWorkspace(), State: st.GetState(), PermissionMode: st.GetPermissionMode(), RunId: st.GetRunId(), VendorId: st.GetVendorId(), LastSeq: st.GetLastSeq()}
	v.ProjectName = p.GetName()
	v.Alias = s.GetAlias()
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
		v.AuthBackend = a.GetAuthBackend()
	}
	if b := s.GetAuthBinding(); b != nil {
		if b.GetBindingId() == "" {
			var err error
			b, err = c.Bindings.Get(ctx, resource.AuthBindingGetRequest_builder{Ref: resource.AuthBindingRef_builder{Id: b.GetId()}.Build(), Select: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build(), opts...)
			if err != nil {
				return nil, err
			}
		}
		v.AuthBinding = b.GetBindingId()
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
	if r.Account == "" {
		return nil, ErrAccountRequired
	}
	p, err := c.projects.Add(ctx, resource.ProjectAddRequest_builder{Workspace: r.Workspace}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	if p.GetStatus().GetState() != "running" {
		if _, err = c.projects.Up(ctx, resource.ProjectUpRequest_builder{Ref: pr(p.GetRuntimeId()), ClientId: &r.ClientId, Agent: &r.Agent}.Build(), opts...); err != nil {
			return nil, err
		}
	}
	b, err := c.Bind(ctx, p.GetRuntimeId(), r.Account)
	if err != nil {
		return nil, err
	}
	if (r.AuthBinding != "" && r.AuthBinding != b.GetBindingId()) || (r.AuthBackend != "" && r.AuthBackend != b.GetAuthBackend()) {
		return nil, fmt.Errorf("auth binding/backend mismatch")
	}
	s, err := c.sessions.Add(ctx, resource.SessionAddRequest_builder{Project: pr(p.GetRuntimeId()), Name: r.Title, ClientId: r.ClientId, Agent: r.Agent, Model: r.Model, Account: ar(r.Account), AuthBinding: br(b.GetBindingId())}.Build(), opts...)
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
func (c *Client) Activity(ctx context.Context, r *api.ActivityInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	return receipt(c.sessions.Activity(ctx, resource.SessionActivityRequest_builder{Ref: sr(r.SessionId), RunId: &r.RunId, ClientId: &r.ClientId, Busy: &r.Busy}.Build(), opts...))
}
func (c *Client) UpdateAgent(ctx context.Context, r *api.AgentUpdateInput, opts ...grpc.CallOption) (*api.AgentUpdateStatus, error) {
	v, e := c.sessions.UpdateAgent(ctx, resource.SessionUpdateRequest_builder{Ref: sr(r.SessionId), RunId: &r.RunId, Binary: &r.Binary, Apply: &r.Apply}.Build(), opts...)
	if e != nil {
		return nil, e
	}
	return &api.AgentUpdateStatus{Ready: v.GetReady(), Reason: v.GetReason(), Binary: v.GetBinary(), State: v.GetState()}, nil
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
	s, err := c.sessions.Events(ctx, resource.SessionEventsRequest_builder{Ref: sr(r.SessionId), AfterSeq: &r.AfterSeq, ClientId: &r.ClientId}.Build(), opts...)
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

func (c *Client) Permission(ctx context.Context, r *api.PermissionInput, opts ...grpc.CallOption) (*api.Receipt, error) {
	return receipt(c.sessions.Permission(ctx, resource.SessionPermissionRequest_builder{Ref: sr(r.SessionId), RunId: &r.RunId, ClientId: &r.ClientId, Mode: &r.Mode}.Build(), opts...))
}

func (c *Client) Logs(ctx context.Context, r *api.LogsRequest, opts ...grpc.CallOption) (*api.LogsReply, error) {
	v, err := c.sessions.Logs(ctx, resource.SessionLogsRequest_builder{Ref: sr(r.SessionId), Project: &r.Project}.Build(), opts...)
	if err != nil {
		return nil, err
	}
	return &api.LogsReply{Text: v.GetText()}, nil
}
