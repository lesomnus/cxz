package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectstate"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func event(e *api.Event) *resource.SessionEvent {
	v := resource.SessionEvent_builder{RunId: e.RunId, Seq: e.Seq, TimeMs: e.TimeMs, Kind: e.Kind, Text: e.Text, RequestId: e.RequestId, Payload: e.Payload, Heartbeat: e.Heartbeat}.Build()
	if r := e.Response; r != nil {
		v.SetResponse(resource.ResponseMetadata_builder{Model: r.Model, Effort: r.Effort, ModelSource: r.ModelSource, EffortSource: r.EffortSource, TurnId: r.TurnId, Phase: r.Phase, CompletionJson: r.CompletionJson}.Build())
	}
	return v
}
func sessionStatus(v *api.Session) *resource.SessionStatus {
	pending := make([]*resource.SessionEvent, 0, len(v.Pending))
	for _, e := range v.Pending {
		pending = append(pending, event(e))
	}
	return resource.SessionStatus_builder{State: v.State, PermissionMode: v.PermissionMode, RunId: v.RunId, VendorId: v.VendorId, LastSeq: v.LastSeq, Queued: v.Queued, Pending: pending}.Build()
}
func projectStatus(v *api.Project) *resource.ProjectStatus {
	return resource.ProjectStatus_builder{ContainerState: projectstate.Of(v.State), ContainerId: v.ContainerId, RemoteUser: v.RemoteUser, RemoteWorkspace: v.RemoteWorkspace, ProvisionState: v.ProvisionState, ProvisionStep: v.ProvisionStep, ProvisionAttempt: v.ProvisionAttempt, Error: v.Error}.Build()
}

func (s Layer) sync(ctx context.Context) error {
	if s.bound {
		return nil
	}
	s.shared.snapshotMu.Lock()
	defer s.shared.snapshotMu.Unlock()
	// No DB transaction or global mutex is held across Docker/RPC calls.
	projects, sessions, err := s.shared.runtime.ResourceSnapshot(ctx)
	if err != nil {
		return err
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	for _, p := range projects.Projects {
		if p.Id == "" || p.State == "foreign" {
			continue
		}
		if _, err = s.saveProject(ctx, p); err != nil {
			return err
		}
	}
	for _, v := range sessions.Sessions {
		if _, err = s.saveSession(ctx, v, ""); err != nil {
			return err
		}
	}
	return nil
}

// Bootstrap once; ordinary resource reads never reconcile Docker/runtime.
// Keep errors retryable rather than poisoning initialization with sync.Once.
// observeWindow is how stale a container observation may be when a read is
// answered. The 30-second reconciliation loop is a safety net for subscribers,
// not a freshness guarantee: a project page that asks is asking now.
//
// It is a window rather than an observation per read because the answer costs a
// Docker inventory. Polling clients share one look; what they cannot do is read
// a conclusion from minutes ago as though it were current -- which is the whole
// reason the state is derived. See docs/plans/project-lifecycle-api.md.
// A variable so a test can close the window rather than wait out the clock.
var observeWindow = 2 * time.Second

// observeProjects refreshes what is known about every project's container, if
// what is known is older than the window. Sessions are not touched: this is the
// container inventory, which is cheap, and not the per-project session calls.
func (s Layer) observeProjects(ctx context.Context) error {
	if s.bound {
		return nil
	}
	s.shared.observedMu.Lock()
	defer s.shared.observedMu.Unlock()
	if time.Since(s.shared.observedAt) < observeWindow {
		return nil
	}
	projects, err := s.shared.runtime.Projects(ctx, &api.Empty{})
	if err != nil {
		// A look that failed leaves the previous one in place, and leaves the
		// window open so the next read tries again. The projects themselves are
		// still answered from the database.
		return nil
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	for _, p := range projects.Projects {
		if p.Id == "" || p.State == "foreign" {
			continue
		}
		if err = s.observeStatus(ctx, p); err != nil {
			return err
		}
	}
	s.shared.observedAt = time.Now()
	return nil
}

// observeStatus updates what is known about a project's container and nothing
// else. It cannot register a project: a container that exists is not a reason
// for a resource to exist, and a project that was erased must not come back
// because its container was still around when the look happened. Registration
// belongs to Add and to the one-time migration of runtime manifests.
func (s Layer) observeStatus(ctx context.Context, v *api.Project) error {
	srv := s.Next().Project()
	ref := projectRef(v.Id)
	old, err := srv.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if status.Code(err) == codes.NotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if !old.GetListed() {
		return nil
	}
	state := projectStatus(v)
	if proto.Equal(old.GetStatus(), state) {
		return nil
	}
	_, err = srv.Patch(ctx, resource.ProjectPatchRequest_builder{Ref: ref, Status: state, DateUpdatedForce: ptr(true)}.Build())
	return err
}

func (s Layer) ensureSnapshot(ctx context.Context) error {
	if s.bound {
		return nil
	}
	s.shared.initialMu.Lock()
	defer s.shared.initialMu.Unlock()
	if s.shared.initialized {
		return nil
	}
	if err := s.sync(ctx); err != nil {
		return err
	}
	s.shared.initialized = true
	return nil
}

// RefreshSession projects one local journal change without scanning inventory.
func (s Layer) RefreshSession(ctx context.Context, id string) error {
	s.shared.snapshotMu.Lock()
	defer s.shared.snapshotMu.Unlock()
	v, err := s.shared.runtime.Get(ctx, &api.SessionRef{Id: id})
	if err != nil {
		return err
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	old, readErr := s.Next().Session().Get(ctx, resource.SessionGetRequest_builder{Ref: sessionRef(id), Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if readErr == nil {
		state := sessionStatus(v)
		state.SetLastSeq(old.GetStatus().GetLastSeq())
		// Token/chunk cursors travel on Events. They must not invalidate the
		// resource list on every output fragment while state is unchanged.
		if proto.Equal(old.GetStatus(), state) {
			return nil
		}
	}
	_, err = s.saveSession(ctx, v, "")
	return err
}
func (s Layer) saveProject(ctx context.Context, v *api.Project) (*resource.Project, error) {
	srv := s.Next().Project()
	ref := projectRef(v.Id)
	old, err := srv.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	state := projectStatus(v)
	if status.Code(err) == codes.NotFound {
		alias, err := s.alias(ctx, v.Id, v.Name, v.Alias)
		if err != nil {
			return nil, err
		}
		return srv.Add(ctx, resource.ProjectAddRequest_builder{Id: resourceID(7, v.Id), Alias: alias, Name: v.Name, Workspace: v.Workspace, Config: v.Config, RuntimeId: v.Id, Status: state, Listed: ptr(true)}.Build())
	}
	if err != nil {
		return nil, err
	}
	if !old.GetListed() {
		return old, nil
	}
	if proto.Equal(old.GetStatus(), state) && old.GetConfig() == v.Config && old.GetListed() {
		return old, nil
	}
	return srv.Patch(ctx, resource.ProjectPatchRequest_builder{Ref: ref, Config: &v.Config, Status: state, Listed: ptr(true), DateUpdatedForce: ptr(true)}.Build())
}
func (s Layer) saveSession(ctx context.Context, v *api.Session, clientID string) (*resource.Session, error) {
	srv := s.Next().Session()
	ref := sessionRef(v.Id)
	old, err := srv.Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	state := sessionStatus(v)
	if status.Code(err) == codes.NotFound {
		if clientID == "" {
			clientID = v.CreateId
		}
		if clientID == "" {
			clientID = "recovered:" + v.Id
		}
		var account *resource.AccountRef
		if v.Account != "" {
			if err := s.ensureAccount(ctx, v.Account, v.Agent, v.AuthBackend); err != nil {
				return nil, err
			}
			account = accountRef(v.Account)
		}
		binding, err := s.ensureBinding(ctx, v.ProjectId, v.Account, v.Agent, v.AuthBackend)
		if err != nil {
			return nil, err
		}
		if binding.GetBindingId() != v.AuthBinding {
			return nil, status.Error(codes.FailedPrecondition, "runtime auth binding mismatch")
		}
		alias, err := s.newSessionAlias(ctx)
		if err != nil {
			return nil, err
		}
		return srv.Add(ctx, resource.SessionAddRequest_builder{Id: resourceID(8, v.Id), Alias: &alias, Name: v.Title, Project: projectRef(v.ProjectId), Agent: v.Agent, Model: v.Model, Account: account, AuthBinding: bindingRef(v.AuthBinding), RuntimeId: v.Id, ClientId: clientID, DateCreated: timestamppb.New(time.UnixMilli(v.CreatedAt)), Status: state, Listed: ptr(true)}.Build())
	}
	if err != nil {
		return nil, err
	}
	if !old.GetListed() {
		return old, nil
	}
	if old.GetAlias() == "" {
		alias, e := s.newSessionAlias(ctx)
		if e != nil {
			return nil, e
		}
		old, e = srv.Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Alias: &alias, DateUpdatedForce: ptr(true)}.Build())
		if e != nil {
			return nil, e
		}
	}
	if old.GetStatus().GetLastSeq() > state.GetLastSeq() || (old.GetListed() && proto.Equal(old.GetStatus(), state) && old.GetName() == v.Title) {
		return old, nil
	}
	return srv.Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Name: ptr(v.Title), Status: state, Listed: ptr(true), DateUpdatedForce: ptr(true)}.Build())
}
