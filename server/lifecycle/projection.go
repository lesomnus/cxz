package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func event(e *api.Event) *resource.SessionEvent {
	return resource.SessionEvent_builder{RunId: e.RunId, Seq: e.Seq, TimeMs: e.TimeMs, Kind: e.Kind, Text: e.Text, RequestId: e.RequestId, Payload: e.Payload}.Build()
}
func sessionStatus(v *api.Session) *resource.SessionStatus {
	pending := make([]*resource.SessionEvent, 0, len(v.Pending))
	for _, e := range v.Pending {
		pending = append(pending, event(e))
	}
	return resource.SessionStatus_builder{State: v.State, RunId: v.RunId, VendorId: v.VendorId, LastSeq: v.LastSeq, Pending: pending}.Build()
}
func projectStatus(v *api.Project) *resource.ProjectStatus {
	return resource.ProjectStatus_builder{State: v.State, ContainerId: v.ContainerId, RemoteUser: v.RemoteUser, RemoteWorkspace: v.RemoteWorkspace, ProvisionState: v.ProvisionState, ProvisionStep: v.ProvisionStep, ProvisionAttempt: v.ProvisionAttempt, Error: v.Error}.Build()
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
func (s Layer) saveProject(ctx context.Context, v *api.Project) (*resource.Project, error) {
	srv := s.Next().Project()
	ref := projectRef(v.Id)
	old, err := srv.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	state := projectStatus(v)
	if status.Code(err) == codes.NotFound {
		return srv.Add(ctx, resource.ProjectAddRequest_builder{Id: resourceID(7, v.Id), Name: v.Name, Workspace: v.Workspace, Config: v.Config, RuntimeId: v.Id, Status: state, Listed: ptr(true)}.Build())
	}
	if err != nil {
		return nil, err
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
		return srv.Add(ctx, resource.SessionAddRequest_builder{Id: resourceID(8, v.Id), Name: v.Title, Project: projectRef(v.ProjectId), Agent: v.Agent, Model: v.Model, RuntimeId: v.Id, ClientId: clientID, DateCreated: timestamppb.New(time.UnixMilli(v.CreatedAt)), Status: state, Listed: ptr(true)}.Build())
	}
	if err != nil {
		return nil, err
	}
	if old.GetStatus().GetLastSeq() > state.GetLastSeq() || (old.GetListed() && proto.Equal(old.GetStatus(), state)) {
		return old, nil
	}
	return srv.Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Status: state, Listed: ptr(true), DateUpdatedForce: ptr(true)}.Build())
}
