package websandbox

import (
	"context"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (x *Projects) Down(_ context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	s := x.S
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		if r.GetRef() == nil || !matchProject(p, r.GetRef()) {
			continue
		}
		if p.GetStatus() == nil {
			p.SetStatus(resource.ProjectStatus_builder{}.Build())
		}
		p.GetStatus().SetState("stopped")
		p.SetDateUpdated(timestamppb.Now())
		for _, st := range s.sessions {
			if st.value.GetProject().GetRuntimeId() != p.GetRuntimeId() || st.value.GetStatus().GetState() == "stopped" {
				continue
			}
			s.cancelTurn(st)
			s.state(st, "stopped")
		}
		s.projectRev++
		return proto.Clone(p).(*resource.Project), nil
	}
	return nil, status.Error(codes.NotFound, "sandbox project not found")
}
