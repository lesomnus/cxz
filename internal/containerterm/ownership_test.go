//go:build !windows

package containerterm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/projectid"
)

// Inspect carries the actual persisted Docker label. Exec speaks the same
// byte protocols as the real helpers, so rejection is checked before a shell,
// path lookup, download or Wisp process can start.
func helperDocker(t *testing.T, c dockerx.Container) string {
	t.Helper()
	dir := t.TempDir()
	inspect := filepath.Join(dir, "inspect.json")
	raw, err := json.Marshal([]dockerx.Container{c})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(inspect, raw, 0600); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "exec.log")
	script := `#!/bin/sh
if [ "$1" = inspect ]; then cat "$CXZ_OWNERSHIP_TEST_INSPECT"; exit; fi
[ "$1" = exec ] || exit 1
printf '%s\n' "$*" >> "$CXZ_OWNERSHIP_TEST_EXEC"
case "$*" in
  *cxz-paths*) printf 'd\000entry\000\000';;
  *cxz-download*) printf '4\ndata';;
  *wisp)
    printf '{"Version":1}\n'
    while IFS= read -r request; do
      printf '{"Done":true,"Entries":[{"Name":"entry","Kind":"directory","Directory":true}]}\n'
    done;;
  *) printf 'terminal-ready\n';;
esac
`
	if err = os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CXZ_OWNERSHIP_TEST_INSPECT", inspect)
	t.Setenv("CXZ_OWNERSHIP_TEST_EXEC", log)
	return log
}

func TestContainerHelpersUseCanonicalProjectOwnership(t *testing.T) {
	legacy := "0123456789abcdef01234567"
	uuid := projectid.New()
	for _, tc := range []struct {
		name, project, label, owner string
		running, allowed            bool
	}{
		{"migrated project", projectid.Canonical(legacy), legacy, "manager", true, true},
		{"legacy client", legacy, legacy, "manager", true, true},
		{"new UUID project", uuid, uuid, "manager", true, true},
		{"different project", projectid.Canonical(legacy), "abcdef0123456789abcdef01", "manager", true, false},
		{"missing project label", projectid.Canonical(legacy), "", "manager", true, false},
		{"missing owner", projectid.Canonical(legacy), legacy, "", true, false},
		{"stopped container", projectid.Canonical(legacy), legacy, "manager", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c dockerx.Container
			c.ID, c.State.Running = "container", tc.running
			c.Config.Labels = map[string]string{"cxz.owner": tc.owner, "cxz.project": tc.label}
			log := helperDocker(t, c)
			p := &api.Project{Id: tc.project, ContainerId: c.ID, RemoteUser: "developer", RemoteWorkspace: "/workspace"}
			for _, helper := range []string{"terminal", "paths", "download", "wisp"} {
				t.Run(helper, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					var err error
					switch helper {
					case "terminal":
						var terminal Terminal
						terminal, err = OpenPTY(ctx, p, 80, 24)
						if err == nil {
							defer terminal.Close()
							out, _ := io.ReadAll(terminal) // PTYs can return EIO at EOF.
							err = terminal.Wait()
							if !strings.Contains(string(out), "terminal-ready") {
								t.Fatal("terminal did not connect", string(out))
							}
						}
					case "paths":
						var listing PathListing
						listing, err = ListPaths(ctx, p, "/workspace")
						if err == nil && (len(listing.Entries) != 1 || listing.Entries[0].Name != "entry") {
							t.Fatal("path lookup did not run", listing)
						}
					case "download":
						var out bytes.Buffer
						err = DownloadFile(ctx, p, "/workspace/file", &out)
						if err == nil && out.String() != "data" {
							t.Fatal("download did not run", out.String())
						}
					case "wisp":
						var client *wispClient
						client, err = openWisp(ctx, ctx, p)
						if err == nil {
							client.stop()
							select {
							case <-client.done:
							case <-ctx.Done():
								t.Fatal("workspace helper did not stop")
							}
						}
					}
					if (err == nil) != tc.allowed {
						t.Fatal("incorrect container ownership decision", err)
					}
				})
			}
			if !tc.allowed {
				if _, err := os.Stat(log); !os.IsNotExist(err) {
					t.Fatal("rejected container was executed", err)
				}
			}
			if p.Id != tc.project {
				t.Fatal("public project identity was replaced with a Docker label")
			}
		})
	}
}
