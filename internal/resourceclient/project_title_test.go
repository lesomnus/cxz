package resourceclient

import (
	"context"
	"github.com/lesomnus/cxz/resource"
	"google.golang.org/grpc"
	"testing"
)

type titleProjectService struct {
	resource.ProjectServiceClient
	got *resource.ProjectPatchRequest
}

func (p *titleProjectService) Patch(_ context.Context, q *resource.ProjectPatchRequest, _ ...grpc.CallOption) (*resource.Project, error) {
	p.got = q
	return resource.Project_builder{Name: q.GetName()}.Build(), nil
}
func TestRenameProjectUsesTypedNamePatch(t *testing.T) {
	p := &titleProjectService{}
	c := &Client{projects: p}
	if err := c.RenameProject(t.Context(), "project-id", "New title"); err != nil {
		t.Fatal(err)
	}
	if p.got.GetRef().GetRuntimeId() != "project-id" || p.got.GetName() != "New title" || p.got.HasAlias() {
		t.Fatal("incorrect name patch", p.got)
	}
}
