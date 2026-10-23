package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/payday/config"
)

type skillFixture struct {
	fixture
	scopes []string
}

func (f *skillFixture) GetSkills(_ context.Context, r *api.SkillsInput) (*api.SkillsReply, error) {
	f.scopes = append(f.scopes, r.Project)
	on := true
	return &api.SkillsReply{Entries: []*api.SkillEntry{
		{Name: "review", Description: "Review a diff.", Effective: true, Override: &on},
		{Name: "deploy", Description: "Ship a release."},
	}}, nil
}

func (f *skillFixture) SetProjectSkill(_ context.Context, r *api.ProjectSkillInput) (*api.SkillsReply, error) {
	f.scopes = append(f.scopes, r.Project)
	return &api.SkillsReply{Message: r.Name}, nil
}

func skillStack(t *testing.T) (*skillFixture, resource.Server, func()) {
	t.Helper()
	ctx := context.Background()
	db, _, err := (config.DbConfig{Driver: "sqlite3", Dsn: "file:" + filepath.Join(t.TempDir(), "resources.db"), MaxOpenConns: 1}).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := &skillFixture{fixture: fixture{
		p: &api.Project{Id: "project", Workspace: "/work", Name: "work", State: "running"},
		s: &api.Session{Id: "session", ProjectId: "project", Workspace: "/work"},
	}}
	stack, err := Build(ctx, db, f, nil)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err = stack.Project().Add(ctx, resource.ProjectAddRequest_builder{Workspace: f.p.Workspace}.Build()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return f, stack, func() { db.Close() }
}

// `in` scopes the library rather than selecting a row to act on. With one, the
// answer is what that project sees; without one it is the installation's own
// defaults -- which is a scope, not a missing argument, so it is not refused.
// The layer passes it through: resolving a handle is the client's (see
// `projectHandle`) and refusing an unknown one is the runtime's (see
// `TestAnUnknownScopeIsRefused` in internal/workspace).
func TestSkillsScopeIsPassedThrough(t *testing.T) {
	ctx := context.Background()
	f, stack, done := skillStack(t)
	defer done()

	if _, err := layerOf(t, stack).Skill().List(ctx, resource.SkillListRequest_builder{}.Build()); err != nil {
		t.Fatal("the installation's own scope was refused:", err)
	}
	if len(f.scopes) != 1 || f.scopes[0] != "" {
		t.Fatal("an absent scope did not reach the runtime as no project:", f.scopes)
	}

	out, err := layerOf(t, stack).Skill().List(ctx, resource.SkillListRequest_builder{In: ptr("project")}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.scopes) != 2 || f.scopes[1] != "project" {
		t.Fatal("the scope did not reach the runtime as it was given:", f.scopes)
	}

	// A project's own decision has to survive the trip, because absent means
	// inherited and that is a different answer from switched off.
	var review, deploy *resource.SkillEntry
	for _, e := range out.GetEntries() {
		switch e.GetName() {
		case "review":
			review = e
		case "deploy":
			deploy = e
		}
	}
	if review == nil || !review.HasOverride() || !review.GetOverride() {
		t.Fatal("a project's own decision was lost")
	}
	if deploy == nil || deploy.HasOverride() {
		t.Fatal("an inherited entry came back as a decision")
	}
}

// A write names the skill and the project separately, and the project is the
// scope rather than the subject: switching one on for a project reaches the
// runtime with both, and neither can stand in for the other.
func TestEnableNamesTheSkillAndTheScope(t *testing.T) {
	ctx := context.Background()
	f, stack, done := skillStack(t)
	defer done()
	out, err := layerOf(t, stack).Skill().Enable(ctx, resource.SkillEnableRequest_builder{
		Name: ptr("review"), In: ptr("project"), Enabled: ptr(true),
	}.Build())
	if err != nil {
		t.Fatal(err)
	}
	if out.GetMessage() != "review" {
		t.Fatal("the skill did not reach the runtime:", out.GetMessage())
	}
	if len(f.scopes) != 1 || f.scopes[0] != "project" {
		t.Fatal("the scope did not reach the runtime:", f.scopes)
	}
}
