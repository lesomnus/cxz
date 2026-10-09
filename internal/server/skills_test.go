package server

import (
	"encoding/json"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/filemap"
	"github.com/lesomnus/cxz/internal/skillconfig"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The library and who sees what are the installation's. A project runtime has
// neither, so it refuses rather than answering with an empty library, which a
// caller would read as "nothing is registered".
func TestTheLibraryNeedsAManager(t *testing.T) {
	s := &Server{}
	if _, err := s.GetSkills(t.Context(), &api.SkillsInput{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.AddSkill(t.Context(), &api.SkillInput{Name: "review"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.SetSkillDefault(t.Context(), &api.SkillDefaultInput{Name: "review"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.ClearProjectSkill(t.Context(), &api.ClearProjectSkillInput{Project: "p", Name: "review"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.RenderDevcontainer(t.Context(), &api.RenderDevcontainerInput{Handle: "/w"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
}

// A project runtime is the side being told, and SyncSkills is the one skills
// call it does answer: what it stores is already resolved for it, so it keeps
// the bundle as given rather than deciding anything.
func TestSyncSkillsIsWhatAProjectRuntimeAnswers(t *testing.T) {
	root := t.TempDir()
	s := &Server{root: root}
	bundle := filemap.Bundle{Files: []filemap.File{{
		Dst:     "${AGENT_CONFIG_DIR}/skills/review/SKILL.md",
		Content: []byte("---\nname: review\ndescription: Review a diff.\n---\n"),
	}}}
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SyncSkills(t.Context(), &api.SyncSkillsInput{Bundle: data}); err != nil {
		t.Fatal(err)
	}
	got, err := skillconfig.LoadRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 1 || got.Files[0].Dst != bundle.Files[0].Dst {
		t.Fatal("the delivered bundle was not stored as given:", got.Files)
	}
}
