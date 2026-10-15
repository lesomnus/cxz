package websandbox

import (
	"context"
	"github.com/lesomnus/cxz/internal/taskapi"
	"github.com/lesomnus/cxz/resource"
)

func (s *Server) Tasks() resource.TaskServiceServer {
	return taskapi.Service{
		TaskServiceServer: s.tasks,
		Project: func(ctx context.Context, ref *resource.ProjectRef) (*resource.Project, error) {
			return (&Projects{S: s}).Get(ctx, resource.ProjectGetRequest_builder{Ref: ref}.Build())
		},
		Session: func(ctx context.Context, ref *resource.SessionRef) (*resource.Session, error) {
			return (&Sessions{S: s}).Get(ctx, resource.SessionGetRequest_builder{Ref: ref}.Build())
		},
	}
}
