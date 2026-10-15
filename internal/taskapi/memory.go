package taskapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Memory is the sandbox storage adapter, not a second copy of task policy.
// Use it behind Service; its mutation methods are intentionally storage-level.
type Memory struct {
	resource.UnimplementedTaskServiceServer
	mu      sync.Mutex
	rows    map[string]*resource.Task
	actions map[string]string
	changed chan struct{}
}

func NewMemory() *Memory {
	return &Memory{rows: map[string]*resource.Task{}, actions: map[string]string{}, changed: make(chan struct{})}
}
func clone(v *resource.Task) *resource.Task { return proto.Clone(v).(*resource.Task) }
func (m *Memory) notify()                   { close(m.changed); m.changed = make(chan struct{}) }

func (m *Memory) find(ref *resource.TaskRef) (*resource.Task, error) {
	if ref.HasId() {
		if len(ref.GetId()) != 16 {
			return nil, invalid("task id must contain 16 bytes")
		}
		if v := m.rows[string(ref.GetId())]; v != nil {
			return v, nil
		}
	} else if ref.HasAlias() {
		for _, v := range m.rows {
			if v.GetAlias() == ref.GetAlias() && (len(ref.GetProject().GetId()) == 0 || bytes.Equal(v.GetProject().GetId(), ref.GetProject().GetId())) {
				return v, nil
			}
		}
	} else if ref.HasClientId() {
		for _, v := range m.rows {
			if v.GetClientId() == ref.GetClientId() && (len(ref.GetProject().GetId()) == 0 || bytes.Equal(v.GetProject().GetId(), ref.GetProject().GetId())) {
				return v, nil
			}
		}
	} else {
		return nil, invalid("task key is required")
	}
	return nil, status.Error(codes.NotFound, "task not found")
}

func (m *Memory) Add(ctx context.Context, r *resource.TaskAddRequest) (*resource.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.rows {
		if bytes.Equal(v.GetProject().GetId(), r.GetProject().GetId()) && (v.GetClientId() == r.GetClientId() || v.GetAlias() == r.GetAlias()) {
			return nil, status.Error(codes.AlreadyExists, "client_id or alias exists")
		}
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	id[6] = id[6]&15 | 0x80
	id[8] = id[8]&63 | 0x80
	id[9] = 11
	now := timestamppb.Now()
	v := resource.Task_builder{Id: id, Alias: r.GetAlias(), ClientId: r.GetClientId(), Name: r.GetName(), Desc: r.GetDesc(), Project: resource.Project_builder{Id: r.GetProject().GetId()}.Build(), Status: r.GetStatus(), Priority: r.GetPriority(), Labels: r.GetLabels(), SourceSession: r.GetSourceSession(), SourceRun: r.GetSourceRun(), SourceSeq: r.GetSourceSeq(), DateCreated: now, DateUpdated: now}.Build()
	m.rows[string(id)] = clone(v)
	m.actions[string(id)], _ = ctx.Value(actionKey{}).(string)
	m.notify()
	return clone(v), nil
}

func (m *Memory) Get(_ context.Context, r *resource.TaskGetRequest) (*resource.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, err := m.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	result := clone(v)
	if r.GetSelect().GetAll() {
		return result, nil
	}
	// Match generated Get's explicit projection (the key is always returned).
	selected := r.GetSelect().ProtoReflect()
	result.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if f.Name() == "id" {
			return true
		}
		sf := selected.Descriptor().Fields().ByName(f.Name())
		if sf == nil || (sf.Kind() == protoreflect.BoolKind && !selected.Get(sf).Bool()) || (sf.Kind() == protoreflect.MessageKind && !selected.Has(sf)) {
			result.ProtoReflect().Clear(f)
		}
		return true
	})
	return result, nil
}

func (m *Memory) Patch(ctx context.Context, r *resource.TaskPatchRequest) (*resource.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, err := m.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	if !proto.Equal(v.GetDateUpdated(), r.GetDateUpdated()) {
		return nil, status.Error(codes.FailedPrecondition, "task changed since the last read")
	}
	if r.HasAlias() {
		for _, other := range m.rows {
			if other != v && other.GetAlias() == r.GetAlias() && bytes.Equal(other.GetProject().GetId(), v.GetProject().GetId()) {
				return nil, status.Error(codes.AlreadyExists, "alias exists")
			}
		}
		v.SetAlias(r.GetAlias())
	}
	if r.HasName() {
		v.SetName(r.GetName())
	}
	if r.HasDesc() {
		v.SetDesc(r.GetDesc())
	}
	if r.HasPriority() {
		v.SetPriority(r.GetPriority())
	}
	if r.HasStatus() {
		v.SetStatus(r.GetStatus())
	}
	if r.HasAssignee() {
		v.SetAssignee(r.GetAssignee())
	}
	if r.HasAssigneeRun() {
		v.SetAssigneeRun(r.GetAssigneeRun())
	}
	if len(r.GetLabels()) > 0 {
		v.SetLabels(maps.Clone(r.GetLabels()))
	}
	if r.GetClearLabels() {
		v.SetLabels(nil)
	}
	now := time.Now()
	if !now.After(v.GetDateUpdated().AsTime()) {
		now = v.GetDateUpdated().AsTime().Add(time.Nanosecond)
	}
	v.SetDateUpdated(timestamppb.New(now))
	m.actions[string(v.GetId())], _ = ctx.Value(actionKey{}).(string)
	m.notify()
	return clone(v), nil
}

func matches(v *resource.Task, filters []*resource.TaskFilter) bool {
	for _, f := range filters {
		if f.HasProject() && !bytes.Equal(f.GetProject().GetId(), v.GetProject().GetId()) {
			continue
		}
		if f.HasAssignee() && f.GetAssignee() != v.GetAssignee() {
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
			if ref.HasAlias() && ref.GetAlias() != v.GetAlias() {
				continue
			}
			if !ref.HasId() && !ref.HasClientId() && !ref.HasAlias() {
				continue
			}
		}
		return true
	}
	return len(filters) == 0
}

func less(a, b *resource.Task) bool {
	if at, bt := a.GetDateCreated().AsTime(), b.GetDateCreated().AsTime(); !at.Equal(bt) {
		return at.Before(bt)
	}
	return bytes.Compare(a.GetId(), b.GetId()) < 0
}
func (m *Memory) List(_ context.Context, r *resource.TaskListRequest) (*resource.TaskListResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var after *resource.Task
	if r.GetAfter() != "" {
		id, err := base64.RawURLEncoding.DecodeString(r.GetAfter())
		if err != nil || len(id) != 16 || m.rows[string(id)] == nil {
			return nil, invalid("invalid task cursor")
		}
		after = m.rows[string(id)]
	}
	items := make([]*resource.Task, 0)
	for _, v := range m.rows {
		if matches(v, r.GetFilters()) && (after == nil || less(after, v)) {
			items = append(items, clone(v))
		}
	}
	sort.Slice(items, func(i, j int) bool { return less(items[i], items[j]) })
	size := int(r.GetSize())
	if size <= 0 {
		size = 100
	}
	if size > 500 {
		size = 500
	}
	next := ""
	if len(items) > size {
		items = items[:size]
		next = base64.RawURLEncoding.EncodeToString(items[len(items)-1].GetId())
	}
	return resource.TaskListResponse_builder{Items: items, Next: next}.Build(), nil
}

func (m *Memory) Watch(r *resource.TaskWatchRequest, out grpc.ServerStreamingServer[resource.TaskWatchResponse]) error {
	versions := map[string]*timestamppb.Timestamp{}
	seen := map[string]bool{}
	first := true
	for {
		m.mu.Lock()
		changed := m.changed // Subscribe before observing state.
		items := make([]*resource.TaskWatchItem, 0)
		for id, v := range m.rows {
			match := matches(v, r.GetFilters())
			if first && r.GetSkipSnapshot() {
				if match {
					versions[id] = v.GetDateUpdated()
				}
				continue
			}
			if match && !proto.Equal(versions[id], v.GetDateUpdated()) {
				versions[id] = v.GetDateUpdated()
				seen[id] = true
				action := ""
				if !first {
					action = m.actions[id]
				}
				items = append(items, resource.TaskWatchItem_builder{Id: v.GetId(), Value: clone(v), Action: action}.Build())
			} else if !match && seen[id] {
				delete(seen, id)
				delete(versions, id)
				items = append(items, resource.TaskWatchItem_builder{Id: v.GetId(), Action: m.actions[id]}.Build())
			}
		}
		m.mu.Unlock()
		if len(items) > 0 || (first && !r.GetSkipSnapshot()) {
			if err := out.Send(resource.TaskWatchResponse_builder{Items: items}.Build()); err != nil {
				return err
			}
		}
		first = false
		select {
		case <-out.Context().Done():
			return out.Context().Err()
		case <-changed:
		}
	}
}
