package multiclient

import (
	"context"
	"testing"

	"github.com/lesomnus/cxz/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type skillDaemon struct {
	daemon
	project string
	name    string
	enabled bool
	calls   []string
}

func (d *skillDaemon) GetSkills(_ context.Context, in *api.SkillsInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	d.calls, d.project = append(d.calls, "get"), in.Project
	return &api.SkillsReply{}, nil
}

func (d *skillDaemon) SetProjectSkill(_ context.Context, in *api.ProjectSkillInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	d.calls = append(d.calls, "set-project")
	d.project, d.name, d.enabled = in.Project, in.Name, in.Enabled
	return &api.SkillsReply{}, nil
}

func (d *skillDaemon) SetSkillDefault(_ context.Context, in *api.SkillDefaultInput, _ ...grpc.CallOption) (*api.SkillsReply, error) {
	d.calls = append(d.calls, "set-default")
	d.name, d.enabled = in.Name, in.Enabled
	return &api.SkillsReply{}, nil
}

// The project says which installation's library is being changed, read from a
// field rather than parsed out of a payload, and the caller's own request is
// left intact because it may have to be retried.
func TestProjectSkillRoutesByProject(t *testing.T) {
	a, b := &skillDaemon{}, &skillDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "a")
	defer c.Close()
	in := &api.ProjectSkillInput{Project: "b::P", Name: "review", Enabled: true}
	if _, err := c.SetProjectSkill(t.Context(), in); err != nil {
		t.Fatal(err)
	}
	if len(a.calls) != 0 || b.project != "P" || b.name != "review" || !b.enabled {
		t.Fatal("wrong installation, project or value", a.calls, b.project, b.name, b.enabled)
	}
	if in.Project != "b::P" {
		t.Fatal("the caller's request was mutated:", in.Project)
	}
}

// A default has no project to follow, so it goes to the connection being
// looked at. Naming one anyway would mean an installation's default being set
// on whichever installation a project happens to belong to.
func TestSkillDefaultFollowsTheSelectedConnection(t *testing.T) {
	a, b := &skillDaemon{}, &skillDaemon{}
	c := New(t.Context(), []Source{{Name: "a", Client: a}, {Name: "b", Client: b}}, "b")
	defer c.Close()
	if _, err := c.SetSkillDefault(t.Context(), &api.SkillDefaultInput{Name: "review", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if len(a.calls) != 0 || b.name != "review" || !b.enabled {
		t.Fatal("a default did not follow the selected connection", a.calls, b.name)
	}
}

// Delivery runs from a manager to a project container. A frontend connection is
// on the far side of that, so it says so rather than reaching for a manager
// that would deliver to itself.
func TestSyncSkillsIsNotAFrontendCall(t *testing.T) {
	c := New(t.Context(), []Source{{Name: "a", Client: &skillDaemon{}}}, "a")
	defer c.Close()
	if _, err := c.SyncSkills(t.Context(), &api.SyncSkillsInput{}); status.Code(err) != codes.Unimplemented {
		t.Fatal(err)
	}
}
