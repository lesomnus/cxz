package lifecycle

import (
	"context"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/projectid"
	"github.com/lesomnus/cxz/resource"
)

// Build makes the project's image from its workspace and runs the project on
// it. The answer is the project as it is afterwards, observed.
func (s ProjectServer) Build(ctx context.Context, r *resource.ProjectBuildRequest) (*resource.Project, error) {
	s.shared.transition.RLock()
	defer s.shared.transition.RUnlock()
	if err := s.effect(); err != nil {
		return nil, err
	}
	p, err := s.ProjectServiceServer.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
	if err != nil {
		return nil, err
	}
	if _, err = s.shared.runtime.BuildProject(ctx, &api.BuildProjectInput{Workspace: projectid.String(p.GetId()), ClientId: r.GetClientId(), Dockerfile: r.GetDockerfile(), Context: r.GetContext()}); err != nil {
		return nil, err
	}
	return s.Get(ctx, resource.ProjectGetRequest_builder{Ref: r.GetRef(), Select: resource.ProjectSelect_builder{All: ptr(true)}.Build()}.Build())
}
