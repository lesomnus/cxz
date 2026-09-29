package skillconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/filemap"
)

func write(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(Library(root), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func skill(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\nDo the thing.\n"
}

// One library, and each project decides what of it it sees. A skill nobody
// enabled reaches nobody, which is also what keeps the delivered bundle small.
func TestAProjectSeesOnlyWhatItEnabled(t *testing.T) {
	root := t.TempDir()
	write(t, root, "review", skill("review", "Review a diff."))
	write(t, root, "deploy", skill("deploy", "Ship a release."))
	for _, name := range []string{"review", "deploy"} {
		if _, err := Apply(root, Request{Action: "add", Name: name}); err != nil {
			t.Fatal(err)
		}
	}

	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Resolve("alpha"); len(got) != 0 {
		t.Fatal("a newly registered skill reached a project on its own:", got)
	}

	// Global default on, one project opting out.
	if _, err := Apply(root, Request{Action: "enable", Name: "review"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, Request{Action: "disable", Name: "review", Project: "beta"}); err != nil {
		t.Fatal(err)
	}
	// One project opting in to something off by default.
	if _, err := Apply(root, Request{Action: "enable", Name: "deploy", Project: "alpha"}); err != nil {
		t.Fatal(err)
	}
	c, _ = Load(root)
	for project, want := range map[string]string{"alpha": "deploy,review", "beta": "", "gamma": "review"} {
		if got := strings.Join(c.Resolve(project), ","); got != want {
			t.Fatalf("%s resolved to %q, want %q", project, got, want)
		}
	}

	// The view separates a project's own decision from what it inherits.
	for _, e := range c.View("beta").Entries {
		switch e.Name {
		case "review":
			if e.Override == nil || *e.Override || e.Effective {
				t.Fatal("beta's opt-out is not reported as its own decision")
			}
		case "deploy":
			if e.Override != nil || e.Effective {
				t.Fatal("beta should be inheriting deploy, switched off")
			}
		}
	}

	if _, err := Apply(root, Request{Action: "inherit", Name: "review", Project: "beta"}); err != nil {
		t.Fatal(err)
	}
	c, _ = Load(root)
	if got := strings.Join(c.Resolve("beta"), ","); got != "review" {
		t.Fatalf("inherit did not restore the global default: %q", got)
	}
}

// Both agents read their skills from their own configuration directory, which
// cxz gives each session, so one destination serves either of them.
func TestDeliveryTargetsOneDirectoryForBothAgents(t *testing.T) {
	root := t.TempDir()
	write(t, root, "review", skill("review", "Review a diff."))
	if _, err := Apply(root, Request{Action: "add", Name: "review"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, Request{Action: "enable", Name: "review"}); err != nil {
		t.Fatal(err)
	}
	c, _ := Load(root)
	m := c.Mappings("alpha")
	if len(m) != 1 || m[0].Dst != "${AGENT_CONFIG_DIR}/skills/review" || m[0].Agent != "" {
		t.Fatalf("unexpected delivery: %+v", m)
	}
	b, err := filemap.Snapshot(root, m)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(Delivered(b), ","); got != "review" {
		t.Fatalf("bundle carries %q", got)
	}
}

// Turning a skill off has to take it away again, or a project keeps what it
// was once shown. cxz owns this directory; the provider's own skills live
// under a dot-prefixed name and are not ours to remove.
func TestDisablingRemovesWhatWasDelivered(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(t.TempDir(), "config")
	write(t, root, "review", skill("review", "Review a diff."))
	if _, err := Apply(root, Request{Action: "add", Name: "review"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, Request{Action: "enable", Name: "review"}); err != nil {
		t.Fatal(err)
	}
	deliver := func(project string) {
		t.Helper()
		c, _ := Load(root)
		b, err := filemap.Snapshot(root, c.Mappings(project))
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveRuntime(root, b); err != nil {
			t.Fatal(err)
		}
		if err := ApplyRuntime(root, "claude", config, t.TempDir(), t.TempDir()); err != nil {
			t.Fatal(err)
		}
	}
	system := filepath.Join(config, "skills", ".system", "imagegen")
	if err := os.MkdirAll(system, 0700); err != nil {
		t.Fatal(err)
	}
	deliver("alpha")
	if _, err := os.Stat(filepath.Join(config, "skills", "review", "SKILL.md")); err != nil {
		t.Fatal("skill was not delivered:", err)
	}

	if _, err := Apply(root, Request{Action: "disable", Name: "review", Project: "alpha"}); err != nil {
		t.Fatal(err)
	}
	deliver("alpha")
	if _, err := os.Stat(filepath.Join(config, "skills", "review")); !os.IsNotExist(err) {
		t.Fatal("a disabled skill stayed in the session")
	}
	if _, err := os.Stat(system); err != nil {
		t.Fatal("removed the provider's own skills:", err)
	}
}

func TestSkillMustAnswerToTheNameItIsDeliveredUnder(t *testing.T) {
	root := t.TempDir()
	write(t, root, "review", skill("code-review", "Review a diff."))
	if _, err := Apply(root, Request{Action: "add", Name: "review"}); err == nil {
		t.Fatal("registered a skill whose frontmatter names a different directory")
	}
	write(t, root, "nodesc", "---\nname: nodesc\n---\n")
	if _, err := Apply(root, Request{Action: "add", Name: "nodesc"}); err == nil {
		t.Fatal("registered a skill with no description")
	}
	if err := ValidateName(Reserved); err == nil {
		t.Fatal("took the name the provider installs into")
	}
	for _, bad := range []string{"Review", "-review", "review-", "re--view", ""} {
		if err := ValidateName(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
