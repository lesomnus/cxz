package accounts

import (
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolationAndRefresh(t *testing.T) {
	root := t.TempDir()
	personal := []byte(`{"tokens":{"access_token":"synthetic-personal"}}`)
	work := []byte(`{"tokens":{"access_token":"synthetic-work"}}`)
	for alias, b := range map[string][]byte{"personal": personal, "work": work} {
		if err := Install(root, alias, "codex", b); err != nil {
			t.Fatal(err)
		}
	}
	if Config(root, "personal") == Config(root, "work") {
		t.Fatal("shared config")
	}
	b, err := Credential(root, "personal", "codex")
	if err != nil || strings.Contains(string(b), "synthetic-work") {
		t.Fatal("cross-account credentials", err)
	}
	p := filepath.Join(Config(root, "work"), "auth.json")
	if err := core.WriteJSON(p, map[string]any{"tokens": map[string]string{"access_token": "synthetic-refreshed"}}); err != nil {
		t.Fatal(err)
	}
	if err := Install(root, "work", "codex", work); err != nil {
		t.Fatal(err)
	}
	b, _ = Credential(root, "work", "codex")
	if !strings.Contains(string(b), "synthetic-refreshed") {
		t.Fatal("overwrote refreshed project tokens")
	}
	if err := Install(root, "work", "codex", []byte(`{"tokens":{"access_token":"synthetic-relogin"}}`)); err != nil {
		t.Fatal(err)
	}
	b, _ = Credential(root, "work", "codex")
	if !strings.Contains(string(b), "synthetic-relogin") {
		t.Fatal("relogin not installed")
	}
	if err := Install(root, "work", "codex", []byte(`{"OPENAI_API_KEY":"secret"}`)); err == nil {
		t.Fatal("API key fallback")
	}
	b, _ = Credential(root, "work", "codex")
	if !strings.Contains(string(b), "synthetic-relogin") {
		t.Fatal("invalid import damaged profile")
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal("credential permissions", st.Mode())
	}
	if _, err := Credential(root, "missing", "codex"); err == nil {
		t.Fatal("missing profile accepted")
	}
	for _, alias := range []string{"../other", "", "WORK", "a/b"} {
		if err := Install(root, alias, "codex", work); err == nil {
			t.Fatal("unsafe alias", alias)
		}
	}
	if err := Prepare(root, "linked", "codex"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(p, filepath.Join(Config(root, "linked"), "auth.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Credential(root, "linked", "codex"); err == nil {
		t.Fatal("symlink credential accepted")
	}
}
func TestEnvironment(t *testing.T) {
	input := []string{"PATH=/bin", "HOME=/host", "OPENAI_API_KEY=personal", "ANTHROPIC_API_KEY=personal", "CLAUDE_CODE_OAUTH_TOKEN=personal", "CODEX_HOME=/old", "CLAUDE_CONFIG_DIR=/old", "AWS_PROFILE=personal", "XDG_CONFIG_HOME=/host", "DBUS_SESSION_BUS_ADDRESS=host", "TERM=xterm"}
	env := Environment(input, "/state", "work", "codex")
	all := strings.Join(env, "\n")
	for _, bad := range []string{"personal", "/host", "/old", "AWS_PROFILE", "DBUS_SESSION"} {
		if strings.Contains(all, bad) {
			t.Fatalf("inherited %s", bad)
		}
	}
	for _, want := range []string{"PATH=/bin", "TERM=xterm", "CODEX_HOME=/state/accounts/work/config", "HOME=/state/accounts/work/home"} {
		if !strings.Contains(all, want) {
			t.Fatal("missing", want)
		}
	}
	if strings.Contains(all, "CLAUDE_CONFIG_DIR") {
		t.Fatal("other vendor config exposed")
	}
}
