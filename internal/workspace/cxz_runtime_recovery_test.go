package workspace

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/dockerx"
)

// Even when the next Manager rollout is newer or in backoff, the pass must
// examine pending project recovery first. Its ownership check must still win:
// a foreign lease cannot be replaced, released, or hidden by the next rollout.
func TestPendingProjectRecoveryPrecedesNewRolloutsAndTheirBackoff(t *testing.T) {
	previous := cxzupdate.Revision
	cxzupdate.Revision = strings.Repeat("a", 40)
	t.Cleanup(func() { cxzupdate.Revision = previous })
	for _, scenario := range []string{"new-manager", "refused-manager", "failed-manager"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			db, err := sql.Open("sqlite3", filepath.Join(root, "registry.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec("CREATE TABLE projects(id TEXT PRIMARY KEY,data BLOB NOT NULL)"); err != nil {
				t.Fatal(err)
			}
			m := &Manager{DB: db, Root: root, Owner: core.ID()}
			p := &Project{ID: "project", Workspace: "/workspace/project", ContainerID: "container", RemoteUser: "cxz"}
			if err := m.save(t.Context(), p); err != nil {
				t.Fatal(err)
			}
			r := cxzupdate.Release{Revision: cxzupdate.Revision, Ancestors: []string{cxzupdate.Revision}}
			state := cxzupdate.State{Channel: "edge", CheckedAt: time.Now(), Release: &r}
			switch scenario {
			case "new-manager":
				r.Revision = strings.Repeat("b", 40)
			case "refused-manager":
				if err := cxzupdate.RecordAttempt(root, cxzupdate.Attempt{At: time.Now(), Outcome: "refused", Reason: "working session"}); err != nil {
					t.Fatal(err)
				}
			case "failed-manager":
				state.FailedRevision, state.FailedAt = r.Revision, time.Now()
			}
			if err := cxzupdate.Save(root, state); err != nil {
				t.Fatal(err)
			}
			container := dockerx.Container{ID: p.ContainerID}
			container.State.Running = true
			container.Config.Labels = map[string]string{"cxz.owner": m.Owner, "cxz.project": p.ID}
			if err := core.WriteJSON(filepath.Join(root, "container.json"), []dockerx.Container{container}); err != nil {
				t.Fatal(err)
			}
			h := cxzupdate.Health{
				Build:   cxzupdate.Build{Revision: cxzupdate.Revision, Platform: cxzupdate.Current().Platform},
				Lease:   "another-maintenance-owner",
				Runtime: &cxzupdate.RuntimeTransaction{ID: "pending-runtime", State: "starting"},
			}
			if err := core.WriteJSON(filepath.Join(root, "health.json"), h); err != nil {
				t.Fatal(err)
			}
			// A health probe reads transaction metadata without running another
			// expensive readiness sweep. No update/lease mutation is permitted.
			script := `#!/bin/sh
case "$1" in
inspect) cat "$CXZ_RECOVERY_TEST_ROOT/container.json" ;;
exec)
  if [ "$6" != _maintenance ] || [ "$8" != health ]; then exit 1; fi
  cat "$CXZ_RECOVERY_TEST_ROOT/health.json"
  ;;
*) exit 1 ;;
esac
`
			if err := os.WriteFile(filepath.Join(root, "docker"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("CXZ_RECOVERY_TEST_ROOT", root)
			m.cxzUpdateStep(t.Context())
			after, err := cxzupdate.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if after.State != "applying" || after.Reason != "project maintenance: "+p.ID {
				t.Fatalf("next Manager rollout masked pending project ownership: %+v", after)
			}
		})
	}
}
