package websandbox

import (
	"context"
	"strings"
	"unicode"

	"github.com/lesomnus/cxz/internal/sessionalias"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Mirror the public lifecycle edge: Patch renames aliases, while manual titles
// use AuxRun. Neither operation restarts a run or creates transcript events.
func (x *Sessions) Patch(_ context.Context, r *resource.SessionPatchRequest) (*resource.Session, error) {
	allowed := true
	r.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		allowed = f.Name() == "ref" || f.Name() == "alias" || f.Name() == "date_updated"
		return allowed
	})
	if !allowed {
		return nil, status.Error(codes.InvalidArgument, "only alias can be patched")
	}
	if !r.HasAlias() || !sessionalias.Valid(r.GetAlias()) {
		return nil, status.Error(codes.InvalidArgument, sessionalias.Rule)
	}
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	for _, other := range s.sessions {
		if other != st && other.value.GetAlias() == r.GetAlias() {
			return nil, status.Error(codes.AlreadyExists, "session alias is already in use")
		}
	}
	st.value.SetAlias(r.GetAlias())
	st.value.SetDateUpdated(timestamppb.Now())
	s.rev++
	return proto.Clone(st.value).(*resource.Session), nil
}

func (x *Sessions) AuxRun(_ context.Context, r *resource.AuxRunRequest) (*resource.AuxState, error) {
	if len(r.GetKinds()) != 1 || r.GetKinds()[0] != resource.AuxKind_AUX_KIND_TITLE || r.GetText() == "" {
		return nil, status.Error(codes.Unimplemented, "sandbox supports manual titles only")
	}
	title := strings.Join(strings.FieldsFunc(r.GetText(), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
	title = strings.Trim(title, "\"'` ")
	if title == "" {
		return nil, status.Error(codes.InvalidArgument, "title must not be empty")
	}
	if runes := []rune(title); len(runes) > 120 {
		title = string(runes[:117]) + "..."
	}
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.find(r.GetRef())
	if err != nil {
		return nil, err
	}
	st.value.SetName(title)
	st.value.SetDateUpdated(timestamppb.Now())
	s.rev++
	return resource.AuxState_builder{Title: proto.String(title)}.Build(), nil
}
