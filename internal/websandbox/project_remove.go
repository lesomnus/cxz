package websandbox

import (
	"context"
	"slices"
	"strings"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Remove only discards the selected project's in-memory sandbox fixtures.
func (x *Projects) Remove(_ context.Context, r *resource.ProjectRemoveRequest) (*resource.ProjectRemoveReply, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		if r.GetTarget() != p.GetRuntimeId() && r.GetTarget() != p.GetAlias() && r.GetTarget() != "@"+p.GetAlias() {
			continue
		}
		reply := resource.ProjectRemoveReply_builder{Project: proto.Clone(p).(*resource.Project), Removed: proto.Bool(r.GetConfirmed())}.Build()
		if !r.GetConfirmed() {
			return reply, nil
		}
		s.sessions = slices.DeleteFunc(s.sessions, func(st *session) bool {
			if st.value.GetProject().GetRuntimeId() != p.GetRuntimeId() {
				return false
			}
			s.cancelTurn(st)
			for path, upload := range s.uploads {
				if strings.HasPrefix(path, "/cxz/assets/"+st.value.GetRuntimeId()+"/") {
					s.uploadBytes -= int64(len(upload.content))
					delete(s.uploads, path)
				}
			}
			return true
		})
		s.projects = slices.DeleteFunc(s.projects, func(other *resource.Project) bool { return other == p })
		s.projectRev++
		s.rev++
		return reply, nil
	}
	return nil, status.Error(codes.NotFound, "sandbox project not found")
}
