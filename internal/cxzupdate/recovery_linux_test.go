package cxzupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

func TestRecoveryFinalizesHealthyRuntimeAfterBudgetStops(t *testing.T) {
	root, target, release := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, release)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := stopRuntime(ctx, tx.Old); err != nil {
		t.Fatal(err)
	}
	tx.State = "starting"
	if err := core.WriteJSON(runtimeTxPath(root), tx); err != nil {
		t.Fatal(err)
	}
	if err := startRuntime(root, target, target, tx.ID); err != nil {
		t.Fatal(err)
	}
	h, err := waitHealth(ctx, root, release.Revision)
	if err != nil {
		t.Fatal(err)
	}
	fixtureBusy(t, root)
	// Completion-only recovery does not depend on re-executing the candidate.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "recovery.json")
	if err := core.WriteJSON(path, Recovery{Project: "p", Container: "c", Transaction: tx.ID, Status: "stopped", Attempts: RecoveryAttempts}); err != nil {
		t.Fatal(err)
	}
	o := RecoveryObservation{Project: "p", Container: "c", Transaction: tx.ID, Phase: "starting", Available: true, Finalize: RuntimeFinalizationKey(h)}
	if o.Finalize == "" {
		t.Fatal("healthy recorded child was not recognized")
	}
	r, ran, err := RunRecovery(ctx, path, o, time.Now, func(finalize bool) RecoveryResult {
		if !finalize {
			t.Fatal("exhausted repair budget permitted a restart")
		}
		return RuntimeRecoveryResult(ctx, root, FinalizeRuntime(ctx, root))
	})
	if err != nil || !ran || r.Status != "completed" || r.Attempts != RecoveryAttempts {
		t.Fatalf("completion was blocked by repair backoff: %+v, %v", r, err)
	}
	after, err := Call(ctx, root, "status", "")
	if err != nil || after.Build.PID != h.Build.PID || after.Start != h.Start || after.Lease != "" || after.Ready {
		t.Fatalf("completion restarted the runtime or lost its busy session: %+v, %v", after, err)
	}
}

func TestCompletionOnlyRecoveryCannotStartOrRollBackRuntime(t *testing.T) {
	root, target, release := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, release)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := stopRuntime(ctx, tx.Old); err != nil {
		t.Fatal(err)
	}
	tx.State = "starting"
	if err := core.WriteJSON(runtimeTxPath(root), tx); err != nil {
		t.Fatal(err)
	}
	if err := FinalizeRuntime(ctx, root); err == nil {
		t.Fatal("nonexistent target was finalized")
	}
	after, err := readRuntimeTransaction(root)
	if err != nil || after.State != "starting" || after.New.Build.PID != 0 {
		t.Fatalf("completion-only recovery mutated runtime startup: %+v, %v", after, err)
	}
}

func TestMaintenancePreservesTypedReadinessFailures(t *testing.T) {
	root, target, release := runtimeFixture(t)
	prepareRuntimeFixture(t, root, target, release)
	fixtureBusy(t, root)
	err := RecoverRuntime(t.Context(), root)
	failure := RecoveryProblem(err)
	if failure == nil || failure.Class != "waiting" || failure.Code != "not_idle" {
		t.Fatalf("readiness refusal became a failed recovery: %+v", failure)
	}
	if err := os.Remove(filepath.Join(root, "fixture-busy")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixture-sessions.json"), []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	failure = RecoveryProblem(RecoverRuntime(t.Context(), root))
	if failure == nil || failure.Class != "transient" {
		t.Fatalf("broken readiness inspection was mistaken for an idle wait: %+v", failure)
	}
}

func TestCompletionOnlyReleasesLeaseAfterHealthyCheckpoint(t *testing.T) {
	root, target, release := runtimeFixture(t)
	tx := prepareRuntimeFixture(t, root, target, release)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := stopRuntime(ctx, tx.Old); err != nil {
		t.Fatal(err)
	}
	tx.State = "starting"
	if err := core.WriteJSON(runtimeTxPath(root), tx); err != nil {
		t.Fatal(err)
	}
	if err := startRuntime(root, target, target, tx.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := waitHealth(ctx, root, release.Revision); err != nil {
		t.Fatal(err)
	}
	// Simulate the helper dying after its completion checkpoint, before release.
	tx, err := readRuntimeTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	tx.State = "healthy"
	if err := core.WriteJSON(runtimeTxPath(root), tx); err != nil {
		t.Fatal(err)
	}
	h, err := Call(ctx, root, "health", "")
	if err != nil || RuntimeFinalizationKey(h) == "" {
		t.Fatal("completed checkpoint hid a surviving owned lease", err)
	}
	fixtureBusy(t, root)
	if err := FinalizeRuntime(ctx, root); err != nil {
		t.Fatal(err)
	}
	after, err := Call(ctx, root, "health", "")
	if err != nil || after.Lease != "" || after.Build.PID != h.Build.PID || after.Start != h.Start {
		t.Fatalf("release restarted the runtime or retained admission: %+v, %v", after, err)
	}
}
