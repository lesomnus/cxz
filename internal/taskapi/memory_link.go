package taskapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"sort"
	"sync"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MemoryLinks is the sandbox storage adapter for links, not a second copy of
// link policy: every refusal lives in LinkService, which this sits behind.
//
// Watch is left unimplemented. Nothing reads a link stream yet -- the board
// comes with the web surface -- and a stream that answers an empty snapshot
// forever is a worse answer than a refusal.
type MemoryLinks struct {
	resource.UnimplementedTaskLinkServiceServer
	mu   sync.Mutex
	rows map[string]*resource.TaskLink
}

func NewMemoryLinks() *MemoryLinks {
	return &MemoryLinks{rows: map[string]*resource.TaskLink{}}
}

func cloneLink(v *resource.TaskLink) *resource.TaskLink {
	return proto.Clone(v).(*resource.TaskLink)
}

func (m *MemoryLinks) find(ref *resource.TaskLinkRef) (*resource.TaskLink, error) {
	if ref.HasId() {
		if v := m.rows[string(ref.GetId())]; v != nil {
			return v, nil
		}
	} else if ref.HasClientId() {
		for _, v := range m.rows {
			if v.GetClientId() == ref.GetClientId() {
				return v, nil
			}
		}
	} else {
		return nil, invalid("link key is required")
	}
	return nil, status.Error(codes.NotFound, "link not found")
}

func (m *MemoryLinks) Add(_ context.Context, r *resource.TaskLinkAddRequest) (*resource.TaskLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.rows {
		if v.GetClientId() == r.GetClientId() {
			return nil, status.Error(codes.AlreadyExists, "client_id exists")
		}
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	id[6] = id[6]&15 | 0x80
	id[8] = id[8]&63 | 0x80
	id[9] = 13
	now := timestamppb.Now()
	v := resource.TaskLink_builder{
		Id: id, ClientId: r.GetClientId(), Kind: r.GetKind(),
		Project:     resource.Project_builder{Id: r.GetProject().GetId()}.Build(),
		Source:      resource.Task_builder{Id: r.GetSource().GetId()}.Build(),
		Target:      resource.Task_builder{Id: r.GetTarget().GetId()}.Build(),
		DateCreated: now, DateUpdated: now,
	}.Build()
	m.rows[string(id)] = cloneLink(v)
	return cloneLink(v), nil
}

func (m *MemoryLinks) Get(_ context.Context, r *resource.TaskLinkGetRequest) (*resource.TaskLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, err := m.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	return cloneLink(v), nil
}

func (m *MemoryLinks) Erase(_ context.Context, r *resource.TaskLinkRef) (*resource.TaskLinkEraseResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, err := m.find(r)
	if err != nil {
		return resource.TaskLinkEraseResponse_builder{Erased: proto.Bool(false)}.Build(), nil
	}
	delete(m.rows, string(v.GetId()))
	return resource.TaskLinkEraseResponse_builder{Erased: proto.Bool(true)}.Build(), nil
}

func linkMatches(v *resource.TaskLink, filters []*resource.TaskLinkFilter) bool {
	for _, f := range filters {
		if f.HasProject() && !bytes.Equal(f.GetProject().GetId(), v.GetProject().GetId()) {
			continue
		}
		if f.HasSource() && !bytes.Equal(f.GetSource().GetId(), v.GetSource().GetId()) {
			continue
		}
		if f.HasTarget() && !bytes.Equal(f.GetTarget().GetId(), v.GetTarget().GetId()) {
			continue
		}
		if f.HasRef() {
			ref := f.GetRef()
			if ref.HasId() && !bytes.Equal(ref.GetId(), v.GetId()) {
				continue
			}
			if ref.HasClientId() && ref.GetClientId() != v.GetClientId() {
				continue
			}
			if !ref.HasId() && !ref.HasClientId() {
				continue
			}
		}
		return true
	}
	return len(filters) == 0
}

func (m *MemoryLinks) List(_ context.Context, r *resource.TaskLinkListRequest) (*resource.TaskLinkListResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]*resource.TaskLink, 0, len(m.rows))
	for _, v := range m.rows {
		if linkMatches(v, r.GetFilters()) {
			items = append(items, cloneLink(v))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if at, bt := items[i].GetDateCreated().AsTime(), items[j].GetDateCreated().AsTime(); !at.Equal(bt) {
			return at.Before(bt)
		}
		return bytes.Compare(items[i].GetId(), items[j].GetId()) < 0
	})
	size := int(r.GetSize())
	if size <= 0 || size > LinksPerProject {
		size = LinksPerProject
	}
	from := 0
	if r.GetAfter() != "" {
		id, err := base64.RawURLEncoding.DecodeString(r.GetAfter())
		if err != nil {
			return nil, invalid("after must be a cursor from a previous page")
		}
		for i, v := range items {
			if bytes.Equal(v.GetId(), id) {
				from = i + 1
				break
			}
		}
	}
	items = items[min(from, len(items)):]
	out := resource.TaskLinkListResponse_builder{}.Build()
	if len(items) > size {
		items = items[:size]
		out.SetNext(base64.RawURLEncoding.EncodeToString(items[len(items)-1].GetId()))
	}
	out.SetItems(items)
	return out, nil
}
