package websandbox

import (
	"context"
	"sync"

	"github.com/lesomnus/cxz/internal/taskapi"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/protobuf/proto"
)

func (s *Server) project(ctx context.Context, ref *resource.ProjectRef) (*resource.Project, error) {
	return (&Projects{S: s}).Get(ctx, resource.ProjectGetRequest_builder{Ref: ref}.Build())
}

func (s *Server) Tasks() resource.TaskServiceServer {
	return taskapi.Service{
		TaskServiceServer: s.tasks,
		Blockers:          s.links().Blockers,
		Project:           s.project,
		Session: func(ctx context.Context, ref *resource.SessionRef) (*resource.Session, error) {
			return (&Sessions{S: s}).Get(ctx, resource.SessionGetRequest_builder{Ref: ref}.Build())
		},
	}
}

// linkGuard serializes what the real layer serializes with a mutex of its own:
// the read-check-write that refuses a cycle.
var linkGuard sync.Mutex

func (s *Server) links() taskapi.LinkService {
	return taskapi.LinkService{
		TaskLinkServiceServer: s.taskLinks,
		Guard:                 &linkGuard,
		Project:               s.project,
		Task: func(ctx context.Context, ref *resource.TaskRef) (*resource.Task, error) {
			return s.tasks.Get(ctx, resource.TaskGetRequest_builder{
				Ref: ref, Select: resource.TaskSelect_builder{All: proto.Bool(true)}.Build(),
			}.Build())
		},
	}
}

func (s *Server) TaskLinks() resource.TaskLinkServiceServer { return s.links() }
