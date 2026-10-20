package websandbox

import (
	"context"
	"strings"

	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/slug"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (x *Projects) Patch(_ context.Context, r *resource.ProjectPatchRequest) (*resource.Project, error) {
	allowed := true
	r.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		allowed = f.Name() == "ref" || f.Name() == "name" || f.Name() == "alias" || f.Name() == "desc" || f.Name() == "date_updated"
		return allowed
	})
	if !allowed || (!r.HasName() && !r.HasAlias() && !r.HasDesc()) {
		return nil, status.Error(codes.InvalidArgument, "provide a project name, alias or description")
	}
	if r.HasName() && (strings.TrimSpace(r.GetName()) == "" || len(r.GetName()) > 200 || strings.ContainsAny(r.GetName(), "\r\n\t\x00")) {
		return nil, status.Error(codes.InvalidArgument, "display name must be nonempty, at most 200 bytes and contain no control whitespace")
	}
	alias := ""
	if r.HasAlias() {
		var err error
		alias, err = slug.ParseAlias(r.GetAlias())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		if r.GetRef() == nil || !matchProject(p, r.GetRef()) {
			continue
		}
		if r.GetDateUpdated() == nil || !proto.Equal(r.GetDateUpdated(), p.GetDateUpdated()) {
			return nil, status.Error(codes.FailedPrecondition, "project changed; refresh and try again")
		}
		if r.HasAlias() {
			for _, other := range s.projects {
				if other != p && other.GetAlias() == alias {
					return nil, status.Error(codes.AlreadyExists, "project alias is already in use")
				}
			}
			p.SetAlias(alias)
		}
		if r.HasName() {
			p.SetName(r.GetName())
		}
		if r.HasDesc() {
			p.SetDesc(r.GetDesc())
		}
		p.SetDateUpdated(timestamppb.Now())
		s.projectRev++
		return proto.Clone(p).(*resource.Project), nil
	}
	return nil, status.Error(codes.NotFound, "sandbox project not found")
}
