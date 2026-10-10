package websandbox

import (
	"context"

	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/internal/sessiontitle"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Mirror the public lifecycle edge: Patch updates manual titles and aliases
// without restarting a run or creating transcript events.
func (x *Sessions) Patch(_ context.Context, r *resource.SessionPatchRequest) (*resource.Session, error) {
	allowed := true
	r.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		allowed = f.Name() == "ref" || f.Name() == "name" || f.Name() == "alias" || f.Name() == "date_updated"
		return allowed
	})
	if !allowed {
		return nil, status.Error(codes.InvalidArgument, "only name and alias can be patched")
	}
	if !r.HasName() && !r.HasAlias() {
		return nil, status.Error(codes.InvalidArgument, "provide a session name or alias")
	}
	title := sessiontitle.Normalize(r.GetName())
	if r.HasName() && (title == "" || len(r.GetName()) > sessiontitle.MaxInputBytes) {
		return nil, status.Error(codes.InvalidArgument, "invalid session title")
	}
	if r.HasAlias() && !sessionalias.Valid(r.GetAlias()) {
		return nil, status.Error(codes.InvalidArgument, sessionalias.Rule)
	}
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	if r.HasAlias() {
		for _, other := range s.sessions {
			if other != st && other.value.GetAlias() == r.GetAlias() {
				return nil, status.Error(codes.AlreadyExists, "session alias is already in use")
			}
		}
		st.value.SetAlias(r.GetAlias())
	}
	if r.HasName() {
		st.value.SetName(title)
	}
	st.value.SetDateUpdated(timestamppb.Now())
	s.rev++
	return proto.Clone(st.value).(*resource.Session), nil
}
