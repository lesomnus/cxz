package taskapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/pdid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"slices"
)

// The sandbox keeps the same field-level trail shape as payday's recorder.
func (m *Memory) record(ctx context.Context, task *resource.Task, action string, request proto.Message) {
	if named := MutationAction(ctx); named != "" {
		action = named
	}
	fields := []uint32{}
	request.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		name := field.Name()
		if name == "clear_labels" && value.Bool() {
			name = "labels"
		}
		if source := task.ProtoReflect().Descriptor().Fields().ByName(name); source != nil && name != "date_updated" {
			fields = append(fields, uint32(source.Number()))
		}
		return true
	})
	slices.Sort(fields)
	id := pdid.New(3)
	row := resource.Audit_builder{Id: id[:], ObjectId: task.GetId(), Domain: 11, Action: action, Fields: fields, DateCreated: task.GetDateUpdated()}.Build()
	m.trail[string(task.GetId())] = append(m.trail[string(task.GetId())], row)
}

func (m *Memory) History(_ context.Context, request *resource.TaskHistoryRequest) (*resource.AuditListResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, err := m.find(request.GetRef())
	if err != nil {
		return nil, err
	}
	rows := m.trail[string(task.GetId())]
	end := len(rows)
	if request.GetAfter() != "" {
		cursor, err := base64.RawURLEncoding.DecodeString(request.GetAfter())
		if err != nil || len(cursor) != 32 || !bytes.Equal(cursor[:16], task.GetId()) {
			return nil, invalid("invalid history cursor")
		}
		end = -1
		for i, row := range rows {
			if bytes.Equal(row.GetId(), cursor[16:]) {
				end = i
				break
			}
		}
		if end < 0 {
			return nil, invalid("invalid history cursor")
		}
	}
	size := int(request.GetSize())
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	items := []*resource.Audit{}
	for i := end - 1; i >= 0 && len(items) < size; i-- {
		items = append(items, proto.Clone(rows[i]).(*resource.Audit))
	}
	next := ""
	if end > len(items) {
		next = base64.RawURLEncoding.EncodeToString(append(bytes.Clone(task.GetId()), items[len(items)-1].GetId()...))
	}
	return resource.AuditListResponse_builder{Items: items, Next: next}.Build(), nil
}
