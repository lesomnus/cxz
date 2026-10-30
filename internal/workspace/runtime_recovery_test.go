package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
)

func TestRuntimeRecoveryBackoffPrecedesStagingAndHelperDispatch(t *testing.T) {
	for _, status := range []string{"retrying", "stopped"} {
		for _, available := range []bool{false, true} {
			t.Run(status+"/"+map[bool]string{true: "alive", false: "dead"}[available], func(t *testing.T) {
				m := &Manager{Root: t.TempDir()}
				p := &Project{ID: "p", ContainerID: "container"}
				path := m.runtimeRecoveryPath(p)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				before := cxzupdate.Recovery{Project: p.ID, Container: p.ContainerID, Transaction: "tx", Status: status, Attempts: 2, NextAttempt: time.Now().Add(time.Hour)}
				if err := core.WriteJSON(path, before); err != nil {
					t.Fatal(err)
				}
				h := cxzupdate.Health{Ready: true, Runtime: &cxzupdate.RuntimeTransaction{ID: "tx", State: "prepared"}}
				// This release cannot be staged. A paused pass must not even try
				// to validate it or invoke a Docker helper, even with fresh idle.
				r, ran, err := m.recoverRuntime(t.Context(), p, cxzupdate.Release{}, h, available, true)
				if err != nil || ran || r.Attempts != 2 || r.Status != status || !r.NextAttempt.Equal(before.NextAttempt) {
					t.Fatalf("backoff dispatched work: %+v, ran=%v, err=%v", r, ran, err)
				}
			})
		}
	}
}

func TestManualRuntimeRetryPreservesTransactionLeaseAndProjectScope(t *testing.T) {
	root := t.TempDir()
	for _, p := range []Project{{ID: "p", Name: "demo", Workspace: "/workspace/demo"}, {ID: "other", Name: "other"}} {
		if err := os.MkdirAll(filepath.Join(root, "projects", p.ID), 0700); err != nil {
			t.Fatal(err)
		}
		if err := core.WriteJSON(filepath.Join(root, "projects", p.ID, "project.json"), p); err != nil {
			t.Fatal(err)
		}
		if err := core.WriteJSON(filepath.Join(root, "projects", p.ID, "runtime-recovery.json"), cxzupdate.Recovery{Project: p.ID, Container: "container", Transaction: "tx", Status: "stopped", Attempts: 5}); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{
		filepath.Join(root, "projects", "p", "state", "data", "runtime-update.json"),
		filepath.Join(root, "projects", "p", "state", "data", "run", "update-lease.json"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := core.WriteJSON(path, map[string]string{"must": "remain unchanged"}); err != nil {
			t.Fatal(err)
		}
	}
	r, err := RetryRuntimeRecovery(root, "demo")
	if err != nil || r.Attempts != 0 || r.Transaction != "tx" || r.Status != "retrying" {
		t.Fatalf("manual retry did not reset only its budget: %+v, %v", r, err)
	}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), "remain unchanged") {
			t.Fatal("manual retry changed runtime state or lease", path, err)
		}
	}
	other, err := cxzupdate.LoadRecovery(filepath.Join(root, "projects", "other", "runtime-recovery.json"))
	if err != nil || other.Attempts != 5 || other.Status != "stopped" {
		t.Fatalf("another project's budget changed: %+v, %v", other, err)
	}
	if _, err := RetryRuntimeRecovery(root, "../../outside"); err == nil {
		t.Fatal("unknown project reference was accepted")
	}
}
