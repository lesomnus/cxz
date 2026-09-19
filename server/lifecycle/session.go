package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type SessionServer struct {
	Layer
	resource.SessionServiceServer
}

func (s SessionServer) Add(ctx context.Context, r *resource.SessionAddRequest) (*resource.Session, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return nil, err
	}
	if r.HasAlias() || r.HasId() || r.GetRuntimeId() != "" || r.HasStatus() || r.HasDateCreated() || (r.HasListed() && !r.GetListed()) {
		return nil, closed()
	}
	if r.GetClientId() == "" {
		return nil, status.Error(codes.InvalidArgument, "client_id required")
	}
	account := ""
	backend := ""
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
		backend = a.GetAuthBackend()
	}
	p, err := s.Next().Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if !p.GetListed() {
		return nil, status.Error(codes.NotFound, "project deleted")
	}
	if _, err := accounts.Resolve(r.GetAgent(), backend); err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	binding, err := s.Next().AuthBinding().Get(ctx, resource.AuthBindingGetRequest_builder{Ref: r.GetAuthBinding(), Select: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if _, err := accounts.ResolveBinding(r.GetAgent(), backend, p.GetRuntimeId(), account, binding.GetBindingId()); err != nil || binding.GetAuthBackend() != backend {
		return nil, status.Error(codes.InvalidArgument, "auth binding does not match project/account/backend")
	}
	v, err := s.shared.runtime.Create(ctx, &api.CreateRequest{Workspace: p.GetWorkspace(), Title: r.GetName(), Agent: r.GetAgent(), Model: r.GetModel(), ClientId: r.GetClientId(), Account: account, AuthBackend: backend, AuthBinding: binding.GetBindingId()})
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
	if err := s.ensureSnapshot(ctx); err != nil {
		return nil, err
	}
	v, err := s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err == nil && !v.GetListed() {
		return nil, status.Error(codes.NotFound, "session deleted")
	}
	if err != nil {
		return nil, err
	}
	return s.SessionServiceServer.Get(ctx, r)
}
func (s SessionServer) List(ctx context.Context, r *resource.SessionListRequest) (*resource.SessionListResponse, error) {
	if len(r.GetFilters()) == 0 {
		r.SetFilters([]*resource.SessionFilter{resource.SessionFilter_builder{Listed: ptr(true)}.Build()})
	}
	if err := s.ensureSnapshot(ctx); err != nil {
		return nil, err
	}
	page, err := s.SessionServiceServer.List(ctx, r)
	if err != nil {
		return nil, err
	}
	items := make([]*resource.Session, 0, len(page.GetItems()))
	for _, v := range page.GetItems() {
		item, err := s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Id: v.GetId()}.Build(), Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build(), Account: resource.AccountSelect_builder{All: ptr(true)}.Build(), AuthBinding: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	page.SetItems(items)
	return page, nil
}
func (s SessionServer) Watch(r *resource.SessionWatchRequest, stream grpc.ServerStreamingServer[resource.SessionWatchResponse]) error {
	s.shared.watchers.Add(1)
	defer s.shared.watchers.Add(-1)
	if err := s.ensureSnapshot(stream.Context()); err != nil {
		return err
	}
	return s.SessionServiceServer.Watch(r, stream)
}
func (s SessionServer) Patch(ctx context.Context, r *resource.SessionPatchRequest) (*resource.Session, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	allowed := true
	r.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if f.Name() != "ref" && f.Name() != "alias" && f.Name() != "date_updated" {
			allowed = false
		}
		return allowed
	})
	if !allowed {
		return nil, closed()
	}
	if !r.HasAlias() || !sessionalias.Valid(r.GetAlias()) {
		return nil, status.Error(codes.InvalidArgument, "alias must contain 3–7 lowercase English letters")
	}
	if err := s.effect(); err != nil {
		return nil, err
	}
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	v, err := s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if !v.GetListed() {
		return nil, status.Error(codes.NotFound, "session deleted")
	}
	owner, err := s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Alias: ptr(r.GetAlias())}.Build()}.Build())
	if err != nil && status.Code(err) != codes.NotFound {
		return nil, err
	}
	if owner != nil && owner.GetRuntimeId() != v.GetRuntimeId() {
		return nil, status.Error(codes.AlreadyExists, "session alias is already in use")
	}
	r.SetDateUpdatedForce(true)
	return s.SessionServiceServer.Patch(ctx, r)
}
func (s SessionServer) Apply(context.Context, *resource.SessionApplyRequest) (*resource.Session, error) {
	return nil, closed()
}
func (s SessionServer) resolve(ctx context.Context, ref *resource.SessionRef) (*resource.Session, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	v, err := s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
	if err == nil && !v.GetListed() {
		return nil, status.Error(codes.NotFound, "session deleted")
	}
	return v, err
}
func (s SessionServer) Resume(ctx context.Context, r *resource.SessionControl) (*resource.Session, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
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
	if err := s.RefreshSession(ctx, v.GetRuntimeId()); err != nil {
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

func (s SessionServer) Attach(ctx context.Context, r *resource.SessionAttachRequest) (*resource.SessionAttachment, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	a, err := s.shared.runtime.Attach(ctx, &api.AttachmentInput{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), Content: r.GetContent()})
	if err != nil {
		return nil, err
	}
	return resource.SessionAttachment_builder{Path: &a.Path}.Build(), nil
}
func (s SessionServer) Activity(ctx context.Context, r *resource.SessionActivityRequest) (*resource.SessionReceipt, error) {
	v, e := s.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if e != nil {
		return nil, e
	}
	return receipt(s.shared.runtime.Activity(ctx, &api.ActivityInput{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), Busy: r.GetBusy()}))
}
func (s SessionServer) UpdateAgent(ctx context.Context, r *resource.SessionUpdateRequest) (*resource.SessionUpdateStatus, error) {
	v, e := s.resolve(ctx, r.GetRef())
	if e != nil {
		return nil, e
	}
	x, e := s.shared.runtime.UpdateAgent(ctx, &api.AgentUpdateInput{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), Binary: r.GetBinary(), Apply: r.GetApply()})
	if e != nil {
		return nil, e
	}
	return resource.SessionUpdateStatus_builder{Ready: &x.Ready, Reason: &x.Reason, Binary: &x.Binary, State: &x.State}.Build(), nil
}
func (s SessionServer) Reply(ctx context.Context, r *resource.SessionReplyRequest) (*resource.SessionReceipt, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return receipt(s.shared.runtime.Reply(ctx, &api.Answer{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), RequestId: r.GetRequestId(), Allow: r.GetAllow(), AnswersJson: r.GetAnswersJson()}))
}
func (s SessionServer) History(ctx context.Context, r *resource.SessionEventsRequest) (*resource.SessionEventBatch, error) {
	v, err := s.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
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
	layer Layer
	id    string
	last  uint64
}

func (s *eventStream) Send(e *api.Event) error {
	if (e.Kind == "state" || e.Kind == "turn_end" || e.Kind == "approval" || e.Kind == "approval_resolved") && e.Seq > s.last {
		s.layer.shared.snapshotMu.Lock()
		v, err := s.layer.shared.runtime.Get(s.Context(), &api.SessionRef{Id: s.id})
		if err != nil {
			s.layer.shared.snapshotMu.Unlock()
			return err
		}
		s.layer.shared.mu.Lock()
		_, err = s.layer.saveSession(s.Context(), v, "")
		s.layer.shared.mu.Unlock()
		s.layer.shared.snapshotMu.Unlock()
		if err != nil {
			return err
		}
		s.last = v.LastSeq
	}
	return s.ServerStreamingServer.Send(event(e))
}
func (s SessionServer) Events(r *resource.SessionEventsRequest, stream grpc.ServerStreamingServer[resource.SessionEvent]) error {
	v, err := s.resolve(stream.Context(), r.GetRef())
	if err != nil {
		return err
	}
	return s.shared.runtime.Watch(&api.WatchRequest{SessionId: v.GetRuntimeId(), AfterSeq: r.GetAfterSeq(), ClientId: r.GetClientId()}, &eventStream{ServerStreamingServer: stream, layer: s.Layer, id: v.GetRuntimeId(), last: v.GetStatus().GetLastSeq()})
}

func (s SessionServer) Permission(ctx context.Context, r *resource.SessionPermissionRequest) (*resource.SessionReceipt, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return receipt(s.shared.runtime.Permission(ctx, &api.PermissionInput{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), Mode: r.GetMode()}))
}
