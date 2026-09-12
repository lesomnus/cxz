package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SessionServer struct {
	Layer
	resource.SessionServiceServer
}

func (s SessionServer) Add(ctx context.Context, r *resource.SessionAddRequest) (*resource.Session, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	if r.HasId() || r.GetRuntimeId() != "" || r.HasStatus() || r.HasDateCreated() || (r.HasListed() && !r.GetListed()) {
		return nil, closed()
	}
	if r.GetClientId() == "" {
		return nil, status.Error(codes.InvalidArgument, "client_id required")
	}
	account := ""
	if r.HasAccount() {
		a, err := s.Next().Account().Get(ctx, resource.AccountGetRequest_builder{Ref: r.GetAccount(), Select: resource.AccountSelect_builder{All: ptr(true)}.Build()}.Build())
		if err != nil {
			return nil, err
		}
		if r.GetAgent() != "" && r.GetAgent() != a.GetAgent() {
			return nil, status.Error(codes.InvalidArgument, "agent does not match account")
		}
		r.SetAgent(a.GetAgent())
		account = a.GetAlias()
	}
	p, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	v, err := s.shared.runtime.Create(ctx, &api.CreateRequest{Workspace: p.GetWorkspace(), Title: r.GetName(), Agent: r.GetAgent(), Model: r.GetModel(), ClientId: r.GetClientId(), Account: account})
	if err != nil {
		return nil, err
	}
	v.ProjectId = p.GetRuntimeId()
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	saved, err := s.saveSession(ctx, v, r.GetClientId())
	if err != nil {
		return nil, err
	}
	if r.GetDesc() != "" {
		return s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(v.Id), Desc: ptr(r.GetDesc()), DateUpdatedForce: ptr(true)}.Build())
	}
	return saved, nil
}
func (s SessionServer) Get(ctx context.Context, r *resource.SessionGetRequest) (*resource.Session, error) {
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	return s.SessionServiceServer.Get(ctx, r)
}
func (s SessionServer) List(ctx context.Context, r *resource.SessionListRequest) (*resource.SessionListResponse, error) {
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	return s.SessionServiceServer.List(ctx, r)
}
func (s SessionServer) Watch(r *resource.SessionWatchRequest, stream grpc.ServerStreamingServer[resource.SessionWatchResponse]) error {
	s.shared.watchers.Add(1)
	defer s.shared.watchers.Add(-1)
	if err := s.sync(stream.Context()); err != nil {
		return err
	}
	return s.SessionServiceServer.Watch(r, stream)
}
func (s SessionServer) Patch(context.Context, *resource.SessionPatchRequest) (*resource.Session, error) {
	return nil, closed()
}
func (s SessionServer) Apply(context.Context, *resource.SessionApplyRequest) (*resource.Session, error) {
	return nil, closed()
}
func (s SessionServer) Erase(context.Context, *resource.SessionRef) (*resource.SessionEraseResponse, error) {
	return nil, closed()
}
func (s SessionServer) resolve(ctx context.Context, ref *resource.SessionRef) (*resource.Session, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	return s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
}
func (s SessionServer) Resume(ctx context.Context, r *resource.SessionControl) (*resource.Session, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.Resume(ctx, &api.Control{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId()})
	if err != nil {
		return nil, err
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	return s.saveSession(ctx, out, "")
}
func (s SessionServer) Stop(ctx context.Context, r *resource.SessionControl) (*resource.Session, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	if _, err = s.shared.runtime.Stop(ctx, &api.Control{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId()}); err != nil {
		return nil, err
	}
	return s.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
}
func receipt(v *api.Receipt, err error) (*resource.SessionReceipt, error) {
	if err != nil {
		return nil, err
	}
	return resource.SessionReceipt_builder{ClientId: &v.ClientId, Status: &v.Status}.Build(), nil
}
func (s SessionServer) Interrupt(ctx context.Context, r *resource.SessionControl) (*resource.SessionReceipt, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return receipt(s.shared.runtime.Interrupt(ctx, &api.Control{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId()}))
}
func (s SessionServer) Send(ctx context.Context, r *resource.SessionSendRequest) (*resource.SessionReceipt, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return receipt(s.shared.runtime.Send(ctx, &api.Input{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), Text: r.GetText()}))
}
func (s SessionServer) Reply(ctx context.Context, r *resource.SessionReplyRequest) (*resource.SessionReceipt, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return receipt(s.shared.runtime.Reply(ctx, &api.Answer{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), RequestId: r.GetRequestId(), Allow: r.GetAllow(), AnswersJson: r.GetAnswersJson()}))
}
func (s SessionServer) History(ctx context.Context, r *resource.SessionEventsRequest) (*resource.SessionEventBatch, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	batch, err := s.shared.runtime.History(ctx, &api.WatchRequest{SessionId: v.GetRuntimeId(), AfterSeq: r.GetAfterSeq()})
	if err != nil {
		return nil, err
	}
	events := make([]*resource.SessionEvent, 0, len(batch.Events))
	for _, e := range batch.Events {
		events = append(events, event(e))
	}
	return resource.SessionEventBatch_builder{Events: events}.Build(), nil
}

type eventStream struct {
	grpc.ServerStreamingServer[resource.SessionEvent]
}

func (s eventStream) Send(e *api.Event) error { return s.ServerStreamingServer.Send(event(e)) }
func (s SessionServer) Events(r *resource.SessionEventsRequest, stream grpc.ServerStreamingServer[resource.SessionEvent]) error {
	v, err := s.resolve(stream.Context(), r.GetRef())
	if err != nil {
		return err
	}
	return s.shared.runtime.Watch(&api.WatchRequest{SessionId: v.GetRuntimeId(), AfterSeq: r.GetAfterSeq()}, eventStream{stream})
}
