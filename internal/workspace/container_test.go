package workspace

import (
	"slices"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/configtrust"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStartingAsksTheCLIForAnExistingContainer(t *testing.T) {
	m := &Manager{Owner: "0123456789abcdef01234567"}
	p := &Project{ID: "project", Workspace: "/workspaces/test"}
	create := m.upArgs(p, "/ws/.devcontainer/devcontainer.json", "/state/devcontainer.json", false)
	start := m.upArgs(p, "/ws/.devcontainer/devcontainer.json", "/state/devcontainer.json", true)
	if slices.Contains(create, "--expect-existing-container") {
		t.Fatal("the create path may not require a container that is not there")
	}
	// The flag is the whole difference: it is what makes the CLI run the
	// start-time hooks only, instead of creating and running everything.
	if !slices.Contains(start, "--expect-existing-container") {
		t.Fatal("the start path may create a container")
	}
	if len(start) != len(create)+1 {
		t.Fatalf("the two paths disagree about more than existence:\n%v\n%v", create, start)
	}
	for _, want := range []string{"--id-label", "cxz.owner=" + m.Owner, "cxz.project=" + p.ID, "--override-config", "/state/devcontainer.json"} {
		if !slices.Contains(start, want) {
			t.Fatalf("missing %q in %v", want, start)
		}
	}
}

func TestStartableRefusesWhatStartingCannotApply(t *testing.T) {
	harmless := map[string]any{"image": "alpine", "postCreateCommand": "go build ./..."}
	seen := &trust{}
	if err := seen.saw("configuration:", harmless); err != nil {
		t.Fatal(err)
	}
	digest := seen.Digest()
	unchanged := rendered{trust: seen}
	if err := unchanged.startable(&Project{ConfigDigest: digest}); err != nil {
		t.Fatal("refused an unchanged configuration", err)
	}
	// Nothing recorded is not a change. These are the projects that existed
	// before cxz kept a digest.
	if err := unchanged.startable(&Project{}); err != nil {
		t.Fatal("reported an unknown digest as a change", err)
	}
	err := unchanged.startable(&Project{ConfigDigest: "22222222"})
	if err == nil || !strings.Contains(err.Error(), "recreate") {
		t.Fatal("a changed configuration was accepted, or did not say what to do", err)
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("a state the caller has to change is not a failure of the operation: %v", status.Code(err))
	}
	// Trust comes first: a configuration that grew something dangerous is a
	// question, not a drift report, and starting is not consent.
	grew := &trust{}
	if err = grew.saw("configuration:", map[string]any{"image": "alpine", "privileged": true}); err != nil {
		t.Fatal(err)
	}
	dangerous := rendered{trust: grew}
	err = dangerous.startable(&Project{ConfigDigest: grew.Digest()})
	if !configtrust.IsRequired(err) {
		t.Fatal("started a configuration nobody said yes to", err)
	}
	if err = dangerous.startable(&Project{ConfigDigest: grew.Digest(), TrustedConfig: grew.Digest()}); err != nil {
		t.Fatal("asked again for a configuration already consented to", err)
	}
}

func TestPreconditionIsNotAFailure(t *testing.T) {
	err := &Precondition{Message: "project has no container to start"}
	s := status.Convert(err)
	if s.Code() != codes.FailedPrecondition || s.Message() != err.Message {
		t.Fatal(s.Code(), s.Message())
	}
}
