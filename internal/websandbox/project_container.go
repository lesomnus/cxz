package websandbox

import (
	"context"
	"github.com/lesomnus/cxz/internal/projectid"

	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The container operations the sandbox can show without a container: what the
// UI reads is the state they leave behind, and the sessions that stop with it.
// See docs/plans/project-lifecycle-api.md.

func (x *Projects) Stop(_ context.Context, r *resource.ProjectControl) (*resource.Project, error) {
	return x.S.container(r, resource.ContainerState_CONTAINER_STATE_STOPPED, true)
}

// Start makes the project run whether or not it had a container, which in the
// sandbox is the same move either way.
func (x *Projects) Start(_ context.Context, r *resource.ProjectStartRequest) (*resource.Project, error) {
	return x.S.container(resource.ProjectControl_builder{Ref: r.GetRef()}.Build(), resource.ContainerState_CONTAINER_STATE_RUNNING, false)
}

// Removing leaves no container, and the project with everything else: the
// sandbox keeps its sessions and their conversations exactly as the real one
// does.
func (x *Projects) Remove(_ context.Context, r *resource.ProjectControl) (*resource.Project, error) {
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
				if projectid.String(st.value.GetProject().GetId()) != projectid.String(p.GetId()) || st.value.GetStatus().GetState() == "stopped" {
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
