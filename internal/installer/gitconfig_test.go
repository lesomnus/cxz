package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/transport"
)

func TestGitConfigSyncPublishesAndContinuesAfterProjectFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[user]\nname=fixture-private-name\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	args := filepath.Join(t.TempDir(), "args")
	t.Setenv("CXZ_TEST_GIT_ARGS", args)
	script := `#!/bin/sh
case "$1" in
 inspect)
  if test "$2" = manager; then
   printf '%s' '[{"Config":{"Labels":{"cxz.owner":"test","cxz.role":"daemon"}}}]'
  else
   printf '%s' '[{"Config":{"Labels":{"cxz.owner":"test","cxz.project":"p"}}}]'
  fi;;
 exec)
  printf '%s\n' "$@" >> "$CXZ_TEST_GIT_ARGS"
  cat >/dev/null
  for arg in "$@"; do if test "$arg" = broken; then exit 1; fi; done;;
 *) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	if err := core.WriteJSON(filepath.Join(root, "installation.json"), transport.Installation{Owner: "test", Container: "manager"}); err != nil {
		t.Fatal(err)
	}
	err := SyncGitConfig(t.Context(), root, nil, &api.Project{Id: "p", ContainerId: "broken"}, &api.Project{Id: "p", ContainerId: "healthy"})
	if err == nil {
		t.Fatal("partial failure reported success")
	}
	logged, _ := os.ReadFile(args)
	if !strings.Contains(string(logged), "healthy") {
		t.Fatal("failure prevented other projects from syncing")
	}
	if strings.Contains(string(logged)+err.Error(), "fixture-private-name") {
		t.Fatal("configuration exposed in argv or diagnostics")
	}
}
