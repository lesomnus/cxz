package filemap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func expect(t *testing.T, path, content string) {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil || string(b) != content {
		t.Fatalf("%s: %q %v", path, b, e)
	}
}
func TestSnapshotAndSessionCopies(t *testing.T) {
	src := t.TempDir()
	root := t.TempDir()
	put(t, filepath.Join(src, "CLAUDE.md"), "shared instructions")
	put(t, filepath.Join(src, "commands", "review.md"), "review")
	mappings := []Mapping{{Src: filepath.Join(src, "CLAUDE.md"), Dst: "${AGENT_CONFIG_DIR}/CLAUDE.md", Agent: "claude"}, {Src: filepath.Join(src, "commands"), Dst: "${AGENT_CONFIG_DIR}/commands", Agent: "claude"}, {Src: filepath.Join(src, "CLAUDE.md"), Dst: "${AGENT_CONFIG_DIR}/AGENTS.md", Agent: "codex"}, {Src: filepath.Join(src, "CLAUDE.md"), Dst: "${SESSION_HOME}/notes"}, {Src: filepath.Join(src, "CLAUDE.md"), Dst: "${WORKSPACE}/notes"}}
	b, e := Snapshot(t.TempDir(), mappings)
	if e != nil {
		t.Fatal(e)
	}
	if e = Save(root, b); e != nil {
		t.Fatal(e)
	}
	// Persisted snapshots survive the source being edited or removed.
	put(t, filepath.Join(src, "CLAUDE.md"), "updated")
	for _, agent := range []string{"claude", "codex"} {
		session := t.TempDir()
		config := filepath.Join(session, "config")
		home := filepath.Join(session, "home")
		work := filepath.Join(session, "workspace")
		if e = Apply(root, agent, config, home, work); e != nil {
			t.Fatal(e)
		}
		name, other := "CLAUDE.md", "AGENTS.md"
		if agent == "codex" {
			name, other = other, name
		}
		expect(t, filepath.Join(config, name), "shared instructions")
		if _, e = os.Stat(filepath.Join(config, other)); !os.IsNotExist(e) {
			t.Fatal("agent filter ignored")
		}
		expect(t, filepath.Join(home, "notes"), "shared instructions")
		expect(t, filepath.Join(work, "notes"), "shared instructions")
		if agent == "claude" {
			expect(t, filepath.Join(config, "commands/review.md"), "review")
		}
		if e = Save(root, Bundle{}); e != nil {
			t.Fatal(e)
		}
		if e = Apply(root, agent, config, home, work); e != nil {
			t.Fatal(e)
		}
		expect(t, filepath.Join(config, name), "shared instructions")
		if e = Save(root, b); e != nil {
			t.Fatal(e)
		}
	}
	b, e = Snapshot(t.TempDir(), mappings)
	if e != nil {
		t.Fatal(e)
	}
	if string(b.Files[0].Content) != "updated" {
		t.Fatal("sync retained stale bytes")
	}
}
func TestRejectInvalidMappings(t *testing.T) {
	for _, dst := range []string{"relative", "${HOME}/CLAUDE.md", "${AGENT_CONFIG_DIR}/../auth", "/tmp/${UNKNOWN}/x", "/", "${WORKSPACE}", "/tmp/x\x00"} {
		if ValidateDestination(dst, "") == nil {
			t.Fatal("accepted", dst)
		}
	}
	src := t.TempDir()
	put(t, filepath.Join(src, "file"), "text")
	if e := os.Symlink(filepath.Join(src, "file"), filepath.Join(src, "link")); e != nil {
		t.Fatal(e)
	}
	if _, e := Snapshot(t.TempDir(), []Mapping{{Src: src, Dst: "${WORKSPACE}/files"}}); e == nil {
		t.Fatal("followed source symlink")
	}
	b := Bundle{Files: []File{{Dst: "${WORKSPACE}/x", Content: []byte(strings.Repeat("x", Limit+1))}}}
	if b.Validate() == nil {
		t.Fatal("unbounded payload")
	}
	b = Bundle{Files: []File{{Dst: "/tmp/x"}, {Dst: "/tmp/x", Agent: "claude"}}}
	if b.Validate() == nil {
		t.Fatal("overlapping agents")
	}
	root := t.TempDir()
	outside := t.TempDir()
	work := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(work, "link")); e != nil {
		t.Fatal(e)
	}
	if e := Save(root, Bundle{Files: []File{{Dst: "${WORKSPACE}/link/x", Content: []byte("bad")}}}); e != nil {
		t.Fatal(e)
	}
	if Apply(root, "claude", t.TempDir(), t.TempDir(), work) == nil {
		t.Fatal("followed destination symlink")
	}
	if _, e := os.Stat(filepath.Join(outside, "x")); !os.IsNotExist(e) {
		t.Fatal("wrote outside destination")
	}
}

func TestDirectoryToAgentConfigRoot(t *testing.T) {
	src := t.TempDir()
	put(t, filepath.Join(src, "CLAUDE.md"), "instructions")
	b, err := Snapshot(t.TempDir(), []Mapping{{Src: src, Dst: "${AGENT_CONFIG_DIR}", Agent: "claude"}})
	if err != nil || len(b.Files) != 1 || b.Files[0].Dst != "${AGENT_CONFIG_DIR}/CLAUDE.md" {
		t.Fatal(b, err)
	}
	if _, err = Snapshot(t.TempDir(), []Mapping{{Src: filepath.Join(src, "CLAUDE.md"), Dst: "${AGENT_CONFIG_DIR}"}}); err == nil {
		t.Fatal("file may not replace config root")
	}
}
