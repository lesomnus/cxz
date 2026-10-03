package hostgit

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotIncludesAndPrecedence(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	write := func(path, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, "xdg/git/config"), "[user]\nname = XDG\n")
	write(filepath.Join(home, ".gitconfig"), "[include]\npath=identity\n[user]\nname=Host\n[alias]\nhello = \"!echo \\\"hello\\\"\"\n[feature]\nflag\n[includeIf \"gitdir:/not-active/\"]\npath=conditional\n")
	write(filepath.Join(home, "identity"), "[user]\nname = Included\nemail=fixture@example.invalid\n[remote \"example\"]\nurl=https://example.invalid/one\nurl=https://example.invalid/two\n")
	write(filepath.Join(home, "conditional"), "[user]\nname = Conditional\n")
	b, err := Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	again, err := Snapshot(t.Context())
	if err != nil || again.ID != b.ID {
		t.Fatal("unchanged snapshot creates another generation", err)
	}
	// Materialize only inside a disposable directory; never read real host config.
	dir := t.TempDir()
	for name, data := range b.Files {
		if err := os.WriteFile(filepath.Join(dir, name), bytes.ReplaceAll(data, []byte(Directory+"/"+b.ID+"/"), []byte(dir+"/")), 0600); err != nil {
			t.Fatal(err)
		}
	}
	get := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"config", "--file", filepath.Join(dir, "config"), "--includes"}, args...)...)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	if get("--get", "user.name") != "Host" || get("--get", "user.email") != "fixture@example.invalid" {
		t.Fatal("precedence/includes lost")
	}
	if get("--get-all", "remote.example.url") != "https://example.invalid/one\nhttps://example.invalid/two" {
		t.Fatal("repeated values lost")
	}
	if get("--bool", "--get", "feature.flag") != "true" {
		t.Fatal("valueless boolean lost")
	}
	if get("--get", "alias.hello") != "!echo \"hello\"" {
		t.Fatal("quoting changed")
	}
	write(filepath.Join(home, "identity"), "[include]\npath=.gitconfig\n")
	if _, err = Snapshot(t.Context()); err == nil {
		t.Fatal("cycle accepted")
	}
}

func TestSnapshotAbsentOverrideAndValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	b, err := Snapshot(t.Context())
	if err != nil || len(b.Files["config"]) != 0 {
		t.Fatal("missing settings must clear managed defaults", err)
	}
	raw, _ := json.Marshal(b)
	if _, err = Read(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	b.Files["../escape"] = nil
	raw, _ = json.Marshal(b)
	if _, err = Read(bytes.NewReader(raw)); err == nil {
		t.Fatal("unsafe archive accepted")
	}
}
