package websandbox

import (
	"context"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The container operations the sandbox can show without a container: what the
// UI reads is the state they leave behind, and the sessions that stop with it.
// See docs/plans/project-lifecycle-api.md.

func (x *Projects) StopContainer(_ context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	return x.S.container(r, resource.ContainerState_CONTAINER_STATE_STOPPED, true)
}

func (x *Projects) StartContainer(_ context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	return x.S.container(r, resource.ContainerState_CONTAINER_STATE_RUNNING, false)
}

// Removing leaves no container, and the project with everything else: the
// sandbox keeps its sessions and their conversations exactly as the real one
// does.
func (x *Projects) RemoveContainer(_ context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	return x.S.container(r, resource.ContainerState_CONTAINER_STATE_ABSENT, true)
}

func (s *Server) container(r *resource.ProjectControl, state resource.ContainerState, stopSessions bool) (*resource.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		if r.GetRef() == nil || !matchProject(p, r.GetRef()) {
			continue
		}
		if p.GetStatus() == nil {
			p.SetStatus(resource.ProjectStatus_builder{}.Build())
		}
		p.GetStatus().SetContainerState(state)
		p.SetDateUpdated(timestamppb.Now())
		if stopSessions {
			for _, st := range s.sessions {
				if st.value.GetProject().GetRuntimeId() != p.GetRuntimeId() || st.value.GetStatus().GetState() == "stopped" {
					continue
				}
				s.cancelTurn(st)
				s.state(st, "stopped")
			}
		}
		s.projectRev++
		return proto.Clone(p).(*resource.Project), nil
	}
	return nil, status.Error(codes.NotFound, "sandbox project not found")
}
