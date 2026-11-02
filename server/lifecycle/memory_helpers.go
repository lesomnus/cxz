package lifecycle

import (
	"bytes"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/dialect/sql/sqlpage"
	"github.com/protobuf-orm/protobuf-orm/ormpatch"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type memoryRouter interface {
	MemoryRoute(context.Context, string, string, proto.Message, proto.Message) (bool, error)
}

func (s MemoryServer) remote(ctx context.Context, p *resource.ProjectRef, method string, r, out proto.Message) (bool, error) {
	if err := s.effect(); err != nil {
		return true, err
	}
	router, ok := s.shared.runtime.(memoryRouter)
	if !ok {
		return false, nil
	}
	project := ""
	if p != nil {
		v, err := s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: p}.Build())
		if err != nil {
			return true, err
		}
		project = projectid.String(v.GetId())
	}
	return router.MemoryRoute(ctx, project, method, r, out)
}
func (s MemoryServer) memoryFilters(ctx context.Context, r *resource.MemoryListRequest) ([]*resource.MemoryFilter, error) {
	var project *resource.ProjectRef
	if r.GetProject() != nil {
		v, err := s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetProject()}.Build())
		if err != nil {
			return nil, err
		}
		project = projectRef(projectid.String(v.GetId()))
	}
	owner := ""
	if ref := r.GetSession(); ref != nil {
		if ref.HasId() {
			if len(ref.GetId()) != 16 {
				return nil, memoryInvalid("invalid session id")
			}
			owner = projectid.String(ref.GetId())
		} else {
			v, err := s.Session().Get(ctx, resource.SessionGetRequest_builder{Ref: ref, Select: resource.SessionSelect_builder{All: ptr(true), Project: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build()}.Build())
			if err != nil {
				return nil, err
			}
			owner = projectid.String(v.GetId())
		}
	}
	filters := r.GetFilters()
	if len(filters) > 32 {
		return nil, memoryInvalid("maximum 32 filters")
	}
	if len(filters) == 0 {
		if project == nil && owner == "" {
			return nil, nil
		}
		filters = []*resource.MemoryFilter{{}}
	}
	out := make([]*resource.MemoryFilter, 0, len(filters))
	for _, f := range filters {
		q := proto.Clone(f).(*resource.MemoryFilter)
		if q.GetProject() != nil {
			v, err := s.Project().Get(ctx, resource.ProjectGetRequest_builder{Ref: q.GetProject()}.Build())
			if err != nil {
				return nil, err
			}
			if project != nil && !bytes.Equal(v.GetId(), project.GetId()) {
				return nil, memoryInvalid("project filters must match the request project")
			}
			q.SetProject(projectRef(projectid.String(v.GetId())))
		} else if project != nil {
			q.SetProject(project)
		}
		if owner != "" {
			if q.HasOwnerSession() && q.GetOwnerSession() != owner {
				return nil, memoryInvalid("owner filter must match session")
			}
			q.SetOwnerSession(owner)
		}
		out = append(out, q)
	}
	return out, nil
}
func (s MemoryServer) memoryListPage(_ context.Context, r *resource.MemoryListRequest, _ []*resource.MemoryFilter, items []*resource.Memory, last *resource.Memory) (*resource.MemoryListResponse, error) {
	id, err := uuid.FromBytes(last.GetId())
	if err != nil {
		return nil, err
	}
	var next string
	if r.GetReverse() {
		next, err = sqlpage.Encode("/reverse", last.GetDateCreated().AsTime(), id)
	} else {
		next, err = sqlpage.Encode(last.GetDateCreated().AsTime(), id)
	}
	return resource.MemoryListResponse_builder{Items: items, Next: next}.Build(), err
}
func (s MemoryServer) patchClearLabels(ctx context.Context, m *resource.Memory, r *resource.MemoryPatchRequest) (*resource.Memory, error) {
	entity := ormpatch.MustEntityOf(resource.File_cxz_memory_proto, "Memory")
	doc, err := ormpatch.FromPatchRequest(entity, r.ProtoReflect(), nil)
	if err != nil {
		return nil, err
	}
	clear := patch.MustNew("cxz.Memory", patch.Target(patch.Num(7)).Assign(patch.Map()))
	doc.GetDelta().SetEntries(append(doc.GetDelta().GetEntries(), clear.GetDelta().GetEntries()...))
	return s.Next().Memory().Apply(ctx, resource.MemoryApplyRequest_builder{Ref: m.Ref(), Patch: doc}.Build())
}
func (s MemoryServer) purgeVersioned(ctx context.Context, m *resource.Memory) (*resource.MemoryEraseResponse, error) {
	driver, tx, err := dialect.BeginTx(ctx, entsql.OpenDB(dialect.SQLite, s.shared.db))
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	store, err := enttx.Rebind(s.Next(), driver)
	if err != nil {
		return nil, err
	}
	current, err := store.Memory().Get(ctx, resource.MemoryGetRequest_builder{Ref: m.Ref(), Select: memoryAll()}.Build())
	if err != nil {
		return nil, err
	}
	if !current.HasDateDeleted() || !proto.Equal(current.GetDateUpdated(), m.GetDateUpdated()) {
		return nil, status.Error(codes.Aborted, "memory changed; refresh before purging")
	}
	out, err := store.Memory().Erase(ctx, m.Ref())
	if err != nil {
		return nil, err
	}
	return out, tx.Commit()
}
func (s MemoryServer) watchMemory(r *resource.MemoryWatchRequest, out grpc.ServerStreamingServer[resource.MemoryWatchResponse]) error {
	previous := map[string]*resource.Memory{}
	first := true
	for {
		current := map[string]*resource.Memory{}
		after := ""
		for {
			p, err := s.List(out.Context(), resource.MemoryListRequest_builder{Project: r.GetProject(), Session: r.GetSession(), Scope: r.GetScope(), Filters: r.GetFilters(), After: after, Size: 500}.Build())
			if err != nil {
				return err
			}
			for _, m := range p.GetItems() {
				current[string(m.GetId())] = m
			}
			after = p.GetNext()
			if after == "" {
				break
			}
		}
		var items []*resource.MemoryWatchItem
		for id, m := range current {
			if !proto.Equal(previous[id], m) {
				items = append(items, resource.MemoryWatchItem_builder{Id: m.GetId(), Value: m}.Build())
			}
		}
		for id, m := range previous {
			if current[id] == nil {
				items = append(items, resource.MemoryWatchItem_builder{Id: m.GetId()}.Build())
			}
		}
		if !(first && r.GetSkipSnapshot()) && (first || len(items) > 0) {
			if err := out.Send(resource.MemoryWatchResponse_builder{Items: items}.Build()); err != nil {
				return err
			}
		}
		previous = current
		first = false
		timer := time.NewTimer(time.Second)
		select {
		case <-out.Context().Done():
			timer.Stop()
			return out.Context().Err()
		case <-timer.C:
		}
	}
}
