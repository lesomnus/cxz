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

// Erase discards the selected project's in-memory sandbox fixtures, which is
// the sandbox's version of deleting what a project owns.
func (x *Projects) Erase(_ context.Context, ref *resource.ProjectRef) (*resource.ProjectEraseResponse, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		if !matchProject(p, ref) {
			continue
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
		return resource.ProjectEraseResponse_builder{Erased: proto.Bool(true)}.Build(), nil
	}
	return nil, status.Error(codes.NotFound, "sandbox project not found")
}
