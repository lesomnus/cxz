package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/devcontainerrender"
	"github.com/lesomnus/xli/xlitest"
)

func renderReply() *api.RenderDevcontainerReply {
	return &api.RenderDevcontainerReply{
		Project: "abc", Name: "demo", Workspace: "/w",
		Files: []*api.RenderedFile{
			{Name: "devcontainer.json", Source: "/state/projects/abc/devcontainer.json", Role: "what cxz passed to the CLI", Data: []byte("{}")},
			{Name: "compose/01-docker-compose.yaml", Source: "/w/.devcontainer/docker-compose.yaml", Role: "from the project", Data: []byte("services: {}\n")},
		},
	}
}

// The directory is the deliverable, so the layout the manager describes has to
// survive to disk: subdirectories included, and an explanation beside them for
// when the terminal that printed it is gone.
func TestRenderedDevcontainerIsWrittenOut(t *testing.T) {
	out := filepath.Join(t.TempDir(), "render")
	reply := renderReply()
	dir, err := writeRenderedDevcontainer(out, reply)
	if err != nil {
		t.Fatal(err)
	}
	if dir != out {
		t.Fatal("ignored the requested directory:", dir)
	}
	for _, f := range reply.Files {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Name)))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != string(f.Data) {
			t.Fatal("wrote different bytes for", f.Name)
		}
	}
	notes, err := os.ReadFile(filepath.Join(dir, "sources.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"demo", "/w", "compose/01-docker-compose.yaml", "/state/projects/abc/devcontainer.json", "from the project"} {
		if !strings.Contains(string(notes), want) {
			t.Fatal("sources.txt does not record", want)
		}
	}
}

// A path from the manager is a path from another machine. It names files in a
// directory this command creates, so it may not reach outside it.
func TestRenderedDevcontainerRefusesEscapingPaths(t *testing.T) {
	for _, name := range []string{"../escape.json", "/etc/passwd", "compose/../../escape"} {
		reply := renderReply()
		reply.Files[0].Name = name
		if _, err := writeRenderedDevcontainer(filepath.Join(t.TempDir(), "render"), reply); err == nil {
			t.Fatal("accepted an unsafe path:", name)
		}
	}
}

// Without files there is nothing to look at, and a directory that says nothing
// is worse than an error that does.
func TestRenderedDevcontainerNeedsFiles(t *testing.T) {
	if _, err := writeRenderedDevcontainer("", &api.RenderDevcontainerReply{}); err == nil {
		t.Fatal("wrote an empty report")
	}
}

// The merged Compose file is the piece people read on its own, so it goes to
// stdout to be piped. Both the file written by render and the file printed here
// are the same one, named in one place.
func TestComposePrintsTheMergedFile(t *testing.T) {
	reply := renderReply()
	merged := []byte("services:\n  dev:\n    image: resolved\n")
	reply.Files = append(reply.Files, &api.RenderedFile{
		Name: devcontainerrender.ResolvedCompose, Source: "docker compose config", Role: "the merge", Data: merged,
	})
	dir, err := writeRenderedDevcontainer(filepath.Join(t.TempDir(), "render"), reply)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(devcontainerrender.ResolvedCompose)))
	if err != nil {
		t.Fatal("render did not write the file the printer names:", err)
	}
	if string(b) != string(merged) {
		t.Fatal("the written file is not what was returned")
	}
}

// Both forms have to survive an absent argument -- reading an absent one with
// MustGet panics rather than returning a zero value, which is how render with
// no argument crashed -- and neither may report success without a manager.
func TestDevcontainerCommandsWithoutAnArgument(t *testing.T) {
	state := filepath.Join(t.TempDir(), "absent")
	for _, args := range [][]string{
		{"devcontainer", "render"}, {"devcontainer", "render", "."},
		{"devcontainer", "docker-compose"}, {"devcontainer", "docker-compose", "."},
	} {
		got := xlitest.Run(t, newRoot(state), args...)
		if got.Err == nil {
			t.Fatal("reported success without a manager:", args)
		}
		if strings.Contains(got.Err.Error(), "arg not set") {
			t.Fatal("absent argument reached the handler as a failure:", got.Err)
		}
	}
}
