package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"sort"
)

func (s *Server) MemoryRoot() (string, bool) { return s.root, s.manager == nil }
func (s *Server) MemoryRoute(ctx context.Context, project, method string, in, out proto.Message) (bool, error) {
	if s.manager == nil {
		return false, nil
	}
	if project != "" {
		return true, s.manager.MemoryInvoke(ctx, project, method, in, out)
	}
	projects, err := s.manager.Projects(ctx)
	if err != nil {
		return true, err
	}
	if r, ok := in.(*resource.MemoryListRequest); ok {
		return true, s.memoryList(ctx, projects.GetProjects(), r, out.(*resource.MemoryListResponse))
	}
	if r, ok := in.(*resource.MemoryChangesRequest); ok {
		return true, s.memoryChanges(ctx, projects.GetProjects(), r, out.(*resource.MemoryChangesReply))
	}
	var ref *resource.MemoryRef
	if r, ok := in.(*resource.MemoryRef); ok {
		ref = r
	} else {
		f := in.ProtoReflect().Descriptor().Fields().ByName("ref")
		if f != nil {
			ref, _ = in.ProtoReflect().Get(f).Message().Interface().(*resource.MemoryRef)
		}
	}
	if ref == nil {
		return true, status.Error(codes.InvalidArgument, "project is required")
	}
	var target string
	for _, p := range projects.GetProjects() {
		if p.Id == "" {
			continue
		}
		probe := &resource.Memory{}
		e := s.manager.MemoryInvoke(ctx, p.Id, resource.MemoryService_Get_FullMethodName, resource.MemoryGetRequest_builder{Ref: ref, Select: resource.MemorySelect_builder{Project: resource.ProjectSelect_builder{All: proto.Bool(true)}.Build()}.Build()}.Build(), probe)
		if status.Code(e) == codes.NotFound {
			continue
		}
		if e != nil {
			return true, e
		}
		if target != "" {
			return true, status.Error(codes.InvalidArgument, "memory reference is ambiguous; specify project")
		}
		target = p.Id
	}
	if target == "" {
		return true, status.Error(codes.NotFound, "memory not found")
	}
	return true, s.manager.MemoryInvoke(ctx, target, method, in, out)
}

// Aggregate only metadata; the returned page keeps a stable date/id order.
func (s *Server) memoryList(ctx context.Context, projects []*api.Project, r *resource.MemoryListRequest, out *resource.MemoryListResponse) error {
	var items []*resource.Memory
	for _, p := range projects {
		if p.Id == "" {
			continue
		}
		request := proto.Clone(r).(*resource.MemoryListRequest)
		request.SetProject(projectid.Ref(p.Id))
		request.SetAfter("")
		request.SetSize(500)
		// A top-level project is the intersection of each filter. Filters for other
		// projects are outside this child's scope and are omitted before forwarding.
		var filters []*resource.MemoryFilter
		for _, f := range r.GetFilters() {
			if f.GetProject() != nil && f.GetProject().HasId() && !bytes.Equal(f.GetProject().GetId(), projectid.Bytes(p.Id)) {
				continue
			}
			filters = append(filters, f)
		}
		if len(r.GetFilters()) > 0 && len(filters) == 0 {
			continue
		}
		request.SetFilters(filters)
		for {
			page := &resource.MemoryListResponse{}
			if err := s.manager.MemoryInvoke(ctx, p.Id, resource.MemoryService_List_FullMethodName, request, page); err != nil {
				return fmt.Errorf("project %s memories: %w", p.Id, err)
			}
			items = append(items, page.GetItems()...)
			if page.GetNext() == "" {
				break
			}
			request.SetAfter(page.GetNext())
		}
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		less := a.GetDateCreated().AsTime().Before(b.GetDateCreated().AsTime())
		if a.GetDateCreated().AsTime().Equal(b.GetDateCreated().AsTime()) {
			less = bytes.Compare(a.GetId(), b.GetId()) < 0
		}
		if r.GetReverse() {
			return !less && !bytes.Equal(a.GetId(), b.GetId())
		}
		return less
	})
	type position struct {
		Time    int64
		ID      []byte
		Reverse bool
	}
	var after position
	if r.GetAfter() != "" {
		b, e := base64.RawURLEncoding.DecodeString(r.GetAfter())
		if e != nil || json.Unmarshal(b, &after) != nil || after.Reverse != r.GetReverse() {
			return status.Error(codes.InvalidArgument, "invalid memory page cursor")
		}
	}
	filtered := items[:0]
	for _, m := range items {
		if r.GetAfter() != "" {
			cmp := m.GetDateCreated().AsTime().UnixNano() - after.Time
			if cmp == 0 {
				cmp = int64(bytes.Compare(m.GetId(), after.ID))
			}
			if (!r.GetReverse() && cmp <= 0) || (r.GetReverse() && cmp >= 0) {
				continue
			}
		}
		filtered = append(filtered, m)
	}
	size := int(r.GetSize())
	if size <= 0 {
		size = 100
	}
	if size > 500 {
		size = 500
	}
	if len(filtered) > size {
		filtered = filtered[:size]
		last := filtered[len(filtered)-1]
		b, _ := json.Marshal(position{last.GetDateCreated().AsTime().UnixNano(), last.GetId(), r.GetReverse()})
		out.SetNext(base64.RawURLEncoding.EncodeToString(b))
	}
	out.SetItems(filtered)
	return nil
}
func (s *Server) memoryChanges(ctx context.Context, projects []*api.Project, r *resource.MemoryChangesRequest, out *resource.MemoryChangesReply) error {
	cursors := map[string]string{}
	if r.GetCursor() != "" {
		b, e := base64.RawURLEncoding.DecodeString(r.GetCursor())
		if e != nil || json.Unmarshal(b, &cursors) != nil {
			return status.Error(codes.InvalidArgument, "invalid connection memory cursor")
		}
	}
	size := int(r.GetSize())
	if size <= 0 {
		size = 100
	}
	if size > 500 {
		size = 500
	}
	for _, p := range projects {
		if p.Id == "" {
			continue
		}
		remaining := size - len(out.GetChanges())
		if remaining == 0 {
			out.SetHasMore(true)
			break
		}
		page := &resource.MemoryChangesReply{}
		err := s.manager.MemoryInvoke(ctx, p.Id, resource.MemoryService_Changes_FullMethodName, resource.MemoryChangesRequest_builder{Project: projectid.Ref(p.Id), Cursor: cursors[p.Id], Size: int32(remaining)}.Build(), page)
		if err != nil {
			return err
		}
		if page.GetResetRequired() {
			out.SetResetRequired(true)
			return nil
		}
		cursors[p.Id] = page.GetCursor()
		out.SetChanges(append(out.GetChanges(), page.GetChanges()...))
		out.SetBaseline(out.GetBaseline() || page.GetBaseline())
		out.SetHasMore(out.GetHasMore() || page.GetHasMore())
	}
	b, _ := json.Marshal(cursors)
	out.SetCursor(base64.RawURLEncoding.EncodeToString(b))
	return nil
}
