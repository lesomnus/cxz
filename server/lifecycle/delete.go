package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

// Journals/volumes are retained. Listed=false is an authoritative tombstone.
// Explicit Project.Add registers a removed workspace again, not its old sessions.
func (s SessionServer) Erase(ctx context.Context, ref *resource.SessionRef) (*resource.SessionEraseResponse, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	s.shared.transition.Lock()
	defer s.shared.transition.Unlock()
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	v, err := s.SessionServiceServer.Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true)}.Build()}.Build())
	if status.Code(err) == codes.NotFound || (err == nil && !v.GetListed()) {
		return &resource.SessionEraseResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	switch v.GetStatus().GetState() {
	case "idle", "working", "waiting_input", "starting":
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if _, err = s.shared.runtime.Stop(ctx, &api.Control{SessionId: v.GetRuntimeId(), RunId: v.GetStatus().GetRunId(), ClientId: core.ID()}); err != nil {
			return nil, err
		}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			current, e := s.shared.runtime.Get(ctx, &api.SessionRef{Id: v.GetRuntimeId()})
			if e != nil {
				return nil, e
			}
			if current.State == "stopped" || current.State == "failed" {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-ticker.C:
			}
		}
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	_, err = s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Listed: ptr(false), AliasNull: ptr(true), DateUpdatedForce: ptr(true)}.Build())
	if err != nil {
		return nil, err
	}
	forgetAuxiliary(ctx, s.shared.runtime, v.GetRuntimeId())
	return resource.SessionEraseResponse_builder{Erased: ptr(true)}.Build(), nil
}

// Erase deletes what a project owns. It is not archiving: the containers, the
// project's volumes and the manager's state directory for it, and every
// session's conversation, profile, attachments and memory go, and the registry
// rows go with them. Workspace sources, shared accounts, shared engine
// resources and other projects are untouched.
//
// It used to mean the opposite -- `listed=false`, everything retained -- while
// the destructive path was `Remove`. Nothing called the archiving one, and a
// name that says erase should erase. See docs/plans/project-lifecycle-api.md.
//
// The runtime finishes its cleanup before the rows go, so a partial failure
// stays retryable: the project is still there to erase again. A repeat call on
// something already gone succeeds.
func (s ProjectServer) Erase(ctx context.Context, ref *resource.ProjectRef) (*resource.ProjectEraseResponse, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	runtime, ok := s.shared.runtime.(interface {
		RemoveProject(context.Context, string, []string) error
	})
	if !ok {
		return nil, status.Error(codes.Unimplemented, "erasing a project requires an updated manager")
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
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if status.Code(err) == codes.NotFound {
		return &resource.ProjectEraseResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	after := ""
	for {
		page, e := s.Next().Session().List(ctx, resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Project: projectRef(projectid.String(p.GetId()))}.Build()}, Size: 200, After: after}.Build())
		if e != nil {
			return nil, e
		}
		for _, v := range page.GetItems() {
			ids = append(ids, v.GetRuntimeId())
		}
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	for _, id := range ids {
		forgetAuxiliary(ctx, s.shared.runtime, id)
	}
	if err = runtime.RemoveProject(ctx, projectid.String(p.GetId()), ids); err != nil {
		return nil, err
	}
	// Publish the disappearance through the watch path before the rows go, so a
	// subscriber removes the item instead of finding a gap. If the transaction
	// below fails, the rows are still here and the retry converges.
	for _, id := range ids {
		if _, err = s.Next().Session().Erase(ctx, sessionRef(id)); err != nil {
			return nil, err
		}
	}
	if _, err = s.Next().Project().Erase(ctx, ref); err != nil {
		return nil, err
	}
	tx, err := s.shared.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, query := range []string{
		"DELETE FROM audit WHERE object_id IN (SELECT id FROM session WHERE project_id IN (SELECT id FROM project WHERE id=?))",
		"DELETE FROM audit WHERE object_id IN (SELECT id FROM authbinding WHERE project_id IN (SELECT id FROM project WHERE id=?))",
		"DELETE FROM audit WHERE object_id IN (SELECT id FROM project WHERE id=?)",
		"DELETE FROM session WHERE project_id IN (SELECT id FROM project WHERE id=?)",
		"DELETE FROM authbinding WHERE project_id IN (SELECT id FROM project WHERE id=?)",
		"DELETE FROM project WHERE id=?",
	} {
		if _, err = tx.ExecContext(ctx, query, projectid.String(p.GetId())); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return resource.ProjectEraseResponse_builder{Erased: ptr(true)}.Build(), nil
}

// A deleted session's aux state goes with it. The call is the manager's own,
// not a client's, which is why it is on the runtime API alone.
func forgetAuxiliary(ctx context.Context, r interface {
	AuxForget(context.Context, *api.AuxForgetInput) (*api.Receipt, error)
}, id string) {
	_, _ = r.AuxForget(ctx, &api.AuxForgetInput{SessionId: id})
}
