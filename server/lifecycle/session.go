package lifecycle

import (
	"context"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/internal/sessiontitle"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	// No project constraint means every listed session, just like List.
	if len(r.GetFilters()) == 0 {
		r.SetFilters([]*resource.SessionFilter{resource.SessionFilter_builder{Listed: ptr(true)}.Build()})
	}
	s.shared.watchers.Add(1)
	defer s.shared.watchers.Add(-1)
	if err := s.ensureSnapshot(stream.Context()); err != nil {
		return err
	}
	return s.watchInventory(r, stream)
}
func (s SessionServer) Patch(ctx context.Context, r *resource.SessionPatchRequest) (*resource.Session, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	allowed := true
	r.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if f.Name() != "ref" && f.Name() != "name" && f.Name() != "alias" && f.Name() != "date_updated" {
			allowed = false
		}
		return allowed
	})
	if !allowed {
		return nil, closed()
	}
	if !r.HasName() && !r.HasAlias() {
		return nil, status.Error(codes.InvalidArgument, "provide a session name or alias")
	}
	if r.HasName() && (len(r.GetName()) > sessiontitle.MaxInputBytes || sessiontitle.Normalize(r.GetName()) == "") {
		return nil, status.Error(codes.InvalidArgument, "invalid session title")
	}
	if r.HasAlias() && !sessionalias.Valid(r.GetAlias()) {
		return nil, status.Error(codes.InvalidArgument, sessionalias.Rule)
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
	if r.HasAlias() {
		owner, err := s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: resource.SessionRef_builder{Alias: ptr(r.GetAlias())}.Build()}.Build())
		if err != nil && status.Code(err) != codes.NotFound {
			return nil, err
		}
		if owner != nil && owner.GetRuntimeId() != v.GetRuntimeId() {
			return nil, status.Error(codes.AlreadyExists, "session alias is already in use")
		}
	}
	if r.HasName() {
		title, err := s.shared.runtime.SetSessionTitle(ctx, v.GetRuntimeId(), r.GetName())
		if err != nil {
			return nil, err
		}
		r.SetName(title)
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
	return receipt(s.shared.runtime.Send(ctx, &api.Input{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), Text: r.GetText(), Cancel: r.GetCancel()}))
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
	x, e := s.shared.runtime.UpdateAgent(ctx, &api.AgentUpdateInput{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), Binary: r.GetBinary(), Apply: r.GetApply(), SupervisorBinary: r.GetSupervisorBinary()})
	if e != nil {
		return nil, e
	}
	return resource.SessionUpdateStatus_builder{Ready: &x.Ready, Reason: &x.Reason, Binary: &x.Binary, State: &x.State, SupervisorBinary: &x.SupervisorBinary, Revision: &x.Revision, Protocol: &x.Protocol}.Build(), nil
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
	batch, err := s.shared.runtime.History(ctx, &api.WatchRequest{SessionId: v.GetRuntimeId(), AfterSeq: r.GetAfterSeq(), Limit: r.GetLimit()})
	if err != nil {
		return nil, err
	}
	events := make([]*resource.SessionEvent, 0, len(batch.Events))
	for _, e := range batch.Events {
		events = append(events, event(e))
	}
	return resource.SessionEventBatch_builder{Events: events}.Build(), nil
}

// Models is served from the runtime's projection; refresh is passed through so a
// client can ask the agent again without a second kind of request.
func (s SessionServer) Models(ctx context.Context, r *resource.SessionModelsRequest) (*resource.SessionModelsReply, error) {
	v, err := s.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.Models(ctx, &api.ModelsRequest{SessionId: v.GetRuntimeId(), Refresh: r.GetRefresh(), RunId: r.GetRunId(), ClientId: r.GetClientId()})
	if err != nil {
		return nil, err
	}
	return resource.SessionModelsReply_builder{LastSeq: &out.LastSeq, Data: out.Data, RunId: &out.RunId,
		CatalogSeq: &out.CatalogSeq, CatalogMs: &out.CatalogMs, Refreshing: &out.Refreshing, Status: &out.Status}.Build(), nil
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
	if err := s.effect(); err != nil {
		return err
	}
	v, err := s.Get(stream.Context(), resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return err
	}
	return s.shared.runtime.Watch(&api.WatchRequest{SessionId: v.GetRuntimeId(), AfterSeq: r.GetAfterSeq(), ClientId: r.GetClientId()}, &eventStream{ServerStreamingServer: stream, layer: s.Layer, id: v.GetRuntimeId(), last: v.GetStatus().GetLastSeq()})
}

// Search passes the question to the runtime, which is the only layer that
// knows where conversations are kept. Nothing about it is per session, so there
// is no ref to resolve and no row to read: the reply is a stream of what was
// found, while it is still being found.
func (s SessionServer) Search(r *resource.SessionSearchRequest, stream grpc.ServerStreamingServer[resource.SessionSearchReply]) error {
	if err := s.effect(); err != nil {
		return err
	}
	return s.shared.runtime.Search(&api.SearchRequest{
		Query: r.GetQuery(), Match: r.GetMatch(), IgnoreCase: r.GetIgnoreCase(),
		IncludeTools: r.GetIncludeTools(),
		SinceMs:      searchBoundMS(r.GetSince()), UntilMs: searchBoundMS(r.GetUntil()),
		Projects: r.GetProjects(), Exclude: r.GetExclude(), Sessions: r.GetSessions(),
		Limit: r.GetLimit(), Snippet: r.GetSnippet(), Cursor: r.GetCursor(), ClientId: r.GetClientId(),
	}, &searchStream{ServerStreamingServer: stream, server: s, ctx: stream.Context(), named: map[string]*resource.Session{}})
}

// The runtime speaks the journal's milliseconds, where an absent bound is zero.
// Nothing is lost in the translation: the window is a range of recorded events,
// and they are recorded to the millisecond.
func searchBoundMS(t *timestamppb.Timestamp) int64 {
	if t == nil {
		return 0
	}
	return t.AsTime().UnixMilli()
}

func searchBound(ms int64) *timestamppb.Timestamp {
	if ms == 0 {
		return nil
	}
	return timestamppb.New(time.UnixMilli(ms).UTC())
}

type searchStream struct {
	grpc.ServerStreamingServer[resource.SessionSearchReply]
	server SessionServer
	ctx    context.Context
	// One lookup per conversation in a page, remembered: a session appears once
	// per page, and the alias a person reads it by lives here, not in the
	// runtime that wrote its journal.
	named map[string]*resource.Session
}

// name adds what this layer owns and the runtime does not: the alias a session
// answers to, the name it was given, and its project's name.
func (s *searchStream) name(runtimeID string) *resource.Session {
	if v, ok := s.named[runtimeID]; ok {
		return v
	}
	v, err := s.server.SessionServiceServer.Get(s.ctx, resource.SessionGetRequest_builder{
		Ref:    resource.SessionRef_builder{RuntimeId: &runtimeID}.Build(),
		Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build(),
	}.Build())
	if err != nil {
		v = nil
	}
	s.named[runtimeID] = v
	return v
}

func (s *searchStream) Send(r *api.SearchReply) error {
	out := resource.SessionSearchReply_builder{}
	if v := r.Visit; v != nil {
		hits := make([]*resource.SessionSearchHit, 0, len(v.Hits))
		for _, h := range v.Hits {
			hits = append(hits, resource.SessionSearchHit_builder{Seq: &h.Seq, TimeMs: &h.TimeMs, Kind: &h.Kind, Bytes: &h.Bytes, Score: &h.Score, Snippet: &h.Snippet}.Build())
		}
		alias, title, project, state := v.Alias, v.Title, v.ProjectName, v.State
		if named := s.name(v.SessionId); named != nil {
			alias = named.GetAlias()
			if named.GetName() != "" {
				title = named.GetName()
			}
			if p := named.GetProject(); p != nil && p.GetName() != "" {
				project = p.GetName()
			}
			state = named.GetStatus().GetState()
		}
		out.Visit = resource.SessionSearchVisit_builder{
			ProjectId: &v.ProjectId, ProjectName: &project,
			SessionId: &v.SessionId, Alias: &alias, Title: &title, Agent: &v.Agent, State: &state,
			ActivityMs: &v.ActivityMs, CreatedMs: &v.CreatedMs, Truncated: &v.Truncated, Hits: hits,
		}.Build()
	}
	if p := r.Progress; p != nil {
		out.Progress = resource.SessionSearchProgress_builder{ProjectId: &p.ProjectId, ProjectName: &p.ProjectName, State: &p.State, Message: &p.Message, Done: &p.Done, Total: &p.Total}.Build()
	}
	if v := r.Summary; v != nil {
		out.Summary = resource.SessionSearchSummary_builder{Projects: &v.Projects, Unavailable: &v.Unavailable, Sessions: &v.Sessions, Hits: &v.Hits, Truncated: &v.Truncated, NextCursor: &v.NextCursor, HasMore: &v.HasMore, Since: searchBound(v.SinceMs), Until: searchBound(v.UntilMs),
			Examined: &v.Examined, Pending: &v.Pending}.Build()
	}
	return s.ServerStreamingServer.Send(out.Build())
}

func (s SessionServer) Background(ctx context.Context, r *resource.SessionBackgroundRequest) (*resource.SessionBackgroundReply, error) {
	v, err := s.Get(ctx, resource.SessionGetRequest_builder{Ref: r.GetRef(), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	reply, err := s.shared.runtime.Background(ctx, &api.SessionRef{Id: v.GetRuntimeId()})
	if err != nil {
		return nil, err
	}
	return resource.SessionBackgroundReply_builder{LastSeq: &reply.LastSeq, Data: reply.Data}.Build(), nil
}

func (s SessionServer) Permission(ctx context.Context, r *resource.SessionPermissionRequest) (*resource.SessionReceipt, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	return receipt(s.shared.runtime.Permission(ctx, &api.PermissionInput{SessionId: v.GetRuntimeId(), RunId: r.GetRunId(), ClientId: r.GetClientId(), Mode: r.GetMode()}))
}

func (s SessionServer) Logs(ctx context.Context, r *resource.SessionLogsRequest) (*resource.SessionLogsReply, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.Logs(ctx, &api.LogsRequest{SessionId: v.GetRuntimeId(), Project: r.GetProject()})
	if err != nil {
		return nil, err
	}
	return resource.SessionLogsReply_builder{Text: &out.Text}.Build(), nil
}

func (s SessionServer) Memory(ctx context.Context, r *resource.SessionMemoryRequest) (*resource.SessionMemoryReply, error) {
	v, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	out, err := s.shared.runtime.Memory(ctx, &api.MemoryRequest{SessionId: v.GetRuntimeId(), Path: r.GetPath()})
	if err != nil {
		return nil, err
	}
	return resource.SessionMemoryReply_builder{Data: out.Data}.Build(), nil
}

func (s SessionServer) CopyMemory(ctx context.Context, r *resource.SessionCopyMemoryRequest) (*resource.SessionReceipt, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	source, err := s.resolve(ctx, r.GetRef())
	if err != nil {
		return nil, err
	}
	target, err := s.resolve(ctx, r.GetTarget())
	if err != nil {
		return nil, err
	}
	return receipt(s.shared.runtime.CopyMemory(ctx, &api.CopyMemoryRequest{SessionId: source.GetRuntimeId(), Path: r.GetPath(), TargetId: target.GetRuntimeId(), TargetPath: r.GetTargetPath()}))
}
