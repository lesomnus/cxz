package cxzupdate

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/core"
)

// FinalizeRuntime never launches or stops a process. It may safely acknowledge
// an already healthy recorded child after the automatic repair budget stops.
func FinalizeRuntime(ctx context.Context, root string) error {
	lock, err := core.Lock(filepath.Join(root, "runtime-update.lock"))
	if err != nil {
		return RecoveryWaiting("helper_running", err.Error())
	}
	defer lock.Close()
	childLock, err := core.Lock(filepath.Join(root, "runtime-start.lock"))
	if err != nil {
		return RecoveryWaiting("child_starting", err.Error())
	}
	defer childLock.Close()
	tx, err := readRuntimeTransaction(root)
	if err != nil {
		return err
	}
	switch tx.State {
	case "starting":
		return finishRuntime(ctx, root, &tx)
	case "rolling_back":
		return finishRuntimeRollback(ctx, root, &tx)
	default:
		return RecoveryWaiting("not_finalizable", "runtime transaction is not awaiting finalization")
	}
}

func RuntimeRecoveryResult(ctx context.Context, root string, err error) RecoveryResult {
	out := RecoveryResult{Outcome: "completed", Failure: RecoveryProblem(err)}
	if out.Failure != nil {
		out.Outcome = "failed"
		if out.Failure.Class == "waiting" {
			out.Outcome = "waiting"
		} else if out.Failure.Class == "operator_required" {
			out.Outcome = "stopped"
		}
	}
	if tx, e := readRuntimeTransaction(root); e == nil {
		out.Transaction, out.Phase = tx.ID, tx.State
		gate := NewGate(root)
		if gate.err == nil && gate.held.ID == tx.ID {
			out.LeaseUntil = gate.held.Until
		}
	}
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, e := Call(probe, root, "health", "")
	out.Available = e == nil
	return out
}

func RuntimeFinalizationKey(h Health) string {
	tx := h.Runtime
	if tx == nil || h.Lease != tx.ID || tx.ID == "" {
		return ""
	}
	var revision string
	switch tx.State {
	case "starting":
		revision = tx.Revision
	case "rolling_back":
		revision = tx.Old.Build.Revision
	default:
		return ""
	}
	if h.Build.Revision != revision || h.Build.PID != tx.New.Build.PID || h.Start != tx.New.Start || h.Build.PID == 0 || h.Start == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%d/%s", tx.ID, tx.State, h.Build.PID, h.Start)
}
