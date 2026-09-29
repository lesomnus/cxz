package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectref"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Remove is distinct from Erase (archive). The runtime finishes destructive
// cleanup before the registry is removed, so partial failures remain retryable.
func (s ProjectServer) Remove(ctx context.Context, r *resource.ProjectRemoveRequest) (*resource.ProjectRemoveReply, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	runtime, ok := s.shared.runtime.(interface {
		RemoveProject(context.Context, string, []string) error
	})
	if !ok {
		return nil, status.Error(codes.Unimplemented, "project removal requires an updated manager")
	}
	if r.GetTarget() == "" {
		return nil, status.Error(codes.InvalidArgument, "project target required")
	}
	s.shared.transition.Lock()
	defer s.shared.transition.Unlock()
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	s.shared.snapshotMu.Lock()
	defer s.shared.snapshotMu.Unlock()
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	// Include archived projects: permanently removing their retained data must not
	// require registering or starting them again.
	var projects []*api.Project
	byID := map[string]*resource.Project{}
	after := ""
	for {
		page, err := s.Next().Project().List(ctx, resource.ProjectListRequest_builder{Size: 200, After: after}.Build())
		if err != nil {
			return nil, err
		}
		for _, p := range page.GetItems() {
			projects = append(projects, &api.Project{Id: p.GetRuntimeId(), Alias: p.GetAlias(), Name: p.GetName(), Workspace: p.GetWorkspace()})
			byID[p.GetRuntimeId()] = p
		}
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	found, err := projectref.Resolve(projects, r.GetTarget())
	if err != nil {
		return nil, err
	}
	p := byID[found.Id]
	if !r.GetConfirmed() {
		return resource.ProjectRemoveReply_builder{Project: p}.Build(), nil
	}
	var ids []string
	after = ""
	for {
		page, err := s.Next().Session().List(ctx, resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Project: projectRef(found.Id)}.Build()}, Size: 200, After: after}.Build())
		if err != nil {
			return nil, err
		}
		for _, v := range page.GetItems() {
			ids = append(ids, v.GetRuntimeId())
		}
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	if err := runtime.RemoveProject(ctx, found.Id, ids); err != nil {
		return nil, err
	}
	// Publish disappearance through the existing watch path before hard deletion.
	// If the transaction fails, unlisted rows still resolve here for a retry.
	for _, id := range ids {
		if _, err := s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(id), Listed: ptr(false), AliasNull: ptr(true), DateUpdatedForce: ptr(true)}.Build()); err != nil {
			return nil, err
		}
	}
	if _, err := s.Next().Project().Patch(ctx, resource.ProjectPatchRequest_builder{Ref: projectRef(found.Id), Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build()); err != nil {
		return nil, err
	}
	tx, err := s.shared.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, query := range []string{
		"DELETE FROM audit WHERE object_id IN (SELECT id FROM session WHERE project_id IN (SELECT id FROM project WHERE runtime_id=?))",
		"DELETE FROM audit WHERE object_id IN (SELECT id FROM authbinding WHERE project_id IN (SELECT id FROM project WHERE runtime_id=?))",
		"DELETE FROM audit WHERE object_id IN (SELECT id FROM project WHERE runtime_id=?)",
		"DELETE FROM session WHERE project_id IN (SELECT id FROM project WHERE runtime_id=?)",
		"DELETE FROM authbinding WHERE project_id IN (SELECT id FROM project WHERE runtime_id=?)",
		"DELETE FROM project WHERE runtime_id=?",
	} {
		if _, err := tx.ExecContext(ctx, query, found.Id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return resource.ProjectRemoveReply_builder{Project: p, Removed: ptr(true)}.Build(), nil
}
