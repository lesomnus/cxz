package lifecycle

import (
	"context"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
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
	_, err = s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: ref, Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build())
	if err != nil {
		return nil, err
	}
	return resource.SessionEraseResponse_builder{Erased: ptr(true)}.Build(), nil
}

func (s ProjectServer) Erase(ctx context.Context, ref *resource.ProjectRef) (*resource.ProjectEraseResponse, error) {
	if err := s.effect(); err != nil {
		return nil, err
	}
	s.shared.transition.Lock()
	defer s.shared.transition.Unlock()
	if err := s.sync(ctx); err != nil {
		return nil, err
	}
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: ref, Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if status.Code(err) == codes.NotFound || (err == nil && !p.GetListed()) {
		return &resource.ProjectEraseResponse{}, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err = s.shared.runtime.Down(ctx, &api.ProjectRequest{Workspace: p.GetRuntimeId(), ClientId: core.ID()}); err != nil {
		return nil, err
	}
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	after := ""
	for {
		page, e := s.Next().Session().List(ctx, resource.SessionListRequest_builder{Filters: []*resource.SessionFilter{resource.SessionFilter_builder{Project: projectRef(p.GetRuntimeId()), Listed: ptr(true)}.Build()}, Size: 200, After: after}.Build())
		if e != nil {
			return nil, e
		}
		for _, v := range page.GetItems() {
			if _, e = s.Next().Session().Patch(ctx, resource.SessionPatchRequest_builder{Ref: sessionRef(v.GetRuntimeId()), Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build()); e != nil {
				return nil, e
			}
		}
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	archivedAlias, e := s.alias(ctx, p.GetRuntimeId(), "deleted-"+p.GetRuntimeId(), "")
	if e != nil {
		return nil, e
	}
	_, err = s.Next().Project().Patch(ctx, resource.ProjectPatchRequest_builder{Ref: ref, Alias: &archivedAlias, Listed: ptr(false), DateUpdatedForce: ptr(true)}.Build())
	if err != nil {
		return nil, err
	}
	return resource.ProjectEraseResponse_builder{Erased: ptr(true)}.Build(), nil
}
