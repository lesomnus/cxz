package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/internal/devcontainerrender"
)

func renderReply() devcontainerrender.Reply {
	return devcontainerrender.Reply{
		Project: "abc", Name: "demo", Workspace: "/w",
		Files: []devcontainerrender.File{
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
	if _, err := writeRenderedDevcontainer("", devcontainerrender.Reply{}); err == nil {
		t.Fatal("wrote an empty report")
	}
}
