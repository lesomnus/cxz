package workspace

import (
	"context"
	"database/sql"
	"github.com/lesomnus/cxz/internal/dockerx"
	_ "github.com/lesomnus/payday/config/dbsqlite3"
	"path/filepath"
	"testing"
)

func TestCheckpointRecoveryAndInventory(t *testing.T) {
	t.Setenv("CXZ_MANAGER_CONTAINER", "")
	root := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(root, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	p := &Project{ID: "project", Workspace: "/workspaces/test", Token: "preserved", Job: ProvisionJob{Attempt: 1}}
	if err = m.checkpoint(context.Background(), p, "devcontainer-up"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DELETE FROM projects"); err != nil {
		t.Fatal(err)
	}
	m, err = New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	p, err = m.resolve(context.Background(), "project")
	if err != nil {
		t.Fatal(err)
	}
	if p.Job.State != "interrupted" || p.Job.Step != "devcontainer-up" || p.Token != "preserved" || p.Job.Attempt != 1 {
		t.Fatalf("lost recovery metadata: %+v", p)
	}
	m.Owner = "owner"
	c := dockerx.Container{ID: "recovered"}
	c.Config.Labels = map[string]string{"cxz.owner": "owner", "cxz.project": "project", "devcontainer.local_folder": p.Workspace}
	if err = m.reconcile(context.Background(), p, []dockerx.Container{c}); err != nil {
		t.Fatal(err)
	}
	if p.ContainerID != c.ID {
		t.Fatal("did not recover resource created before checkpoint")
	}
	other := c
	other.ID = "duplicate"
	if m.reconcile(context.Background(), p, []dockerx.Container{c, other}) == nil {
		t.Fatal("accepted ambiguous ownership")
	}
	c.Config.Labels["cxz.owner"] = "foreign"
	if err = m.reconcile(context.Background(), p, []dockerx.Container{c}); err != nil {
		t.Fatal(err)
	}
	if p.ContainerID != "" {
		t.Fatal("adopted foreign container")
	}
}

func TestSettleRetiresAFailureTheRuntimeDisproved(t *testing.T) {
	const outage = "project runtime not ready: ping: unable to open database file"
	for _, tc := range []struct {
		name    string
		p       Project
		settled bool
		state   string
		step    string
		err     string
	}{
		{
			// cxz#282: the project served every call for an hour while the
			// page kept showing this.
			name:    "failed before the runtime answered",
			p:       Project{Error: outage, Job: ProvisionJob{State: "failed", Step: "runtime-ready", Attempt: 26}},
			settled: true, state: "complete", step: "ready",
		},
		{
			name:    "interrupted while the manager was down",
			p:       Project{Error: "manager stopped during devcontainer-up", Job: ProvisionJob{State: "interrupted", Step: "devcontainer-up"}},
			settled: true, state: "complete", step: "ready",
		},
		{
			name:    "a session that was never created is still missing",
			p:       Project{Error: "no account matched", Job: ProvisionJob{State: "failed", Step: "session"}},
			state:   "failed", step: "session", err: "no account matched",
		},
		{
			name:  "nothing to retire",
			p:     Project{Job: ProvisionJob{State: "complete", Step: "ready"}},
			state: "complete", step: "ready",
		},
		{
			// Down records complete/down; an error left beside it is still
			// stale, but the step it finished is not "ready".
			name:    "an error beside a finished job",
			p:       Project{Error: outage, Job: ProvisionJob{State: "complete", Step: "down"}},
			settled: true, state: "complete", step: "down",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.p
			p.Job.UpdatedAt = 1
			if got := p.settle(); got != tc.settled {
				t.Fatalf("settle() = %v, want %v", got, tc.settled)
			}
			if p.Error != tc.err {
				t.Fatalf("Error = %q, want %q", p.Error, tc.err)
			}
			if p.Job.State != tc.state || p.Job.Step != tc.step {
				t.Fatalf("job = %s/%s, want %s/%s", p.Job.State, p.Job.Step, tc.state, tc.step)
			}
			if p.Job.Attempt != tc.p.Job.Attempt {
				t.Fatalf("lost the attempt count: %d", p.Job.Attempt)
			}
			moved := p.Job.UpdatedAt != 1
			if want := tc.settled && tc.step == "ready"; moved != want {
				t.Fatalf("UpdatedAt moved = %v, want %v", moved, want)
			}
		})
	}
}
