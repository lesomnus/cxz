package cxzupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"time"
)

type RuntimeTransaction struct {
	ID, Target, Revision, State, Error string
	Old, New                           Health
	FailedAt                           time.Time
	Release                            *Release `json:"release,omitempty"`
}

func (tx RuntimeTransaction) Pending() bool {
	return tx.ID != "" && tx.State != "healthy" && tx.State != "rolled_back"
}

func runtimeTxPath(root string) string { return filepath.Join(root, "runtime-update.json") }
func readRuntimeTransaction(root string) (RuntimeTransaction, error) {
	var tx RuntimeTransaction
	b, e := os.ReadFile(runtimeTxPath(root))
	if e == nil {
		e = json.Unmarshal(b, &tx)
	}
	return tx, e
}

// RuntimeChild records its identity before exec or any runtime initialization.
// A helper crash between spawn and acknowledgement cannot create an unknown server.
func RuntimeChild(root, id, target string) error {
	lock, e := core.Lock(filepath.Join(root, "runtime-start.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	tx, e := readRuntimeTransaction(root)
	if e != nil {
		return e
	}
	if tx.ID != id || tx.New.Build.PID != 0 || (tx.State != "starting" && tx.State != "rolling_back") {
		return fmt.Errorf("runtime launch no longer pending")
	}
	expected := tx.Target
	if tx.State == "rolling_back" {
		expected = tx.Old.Binary
	}
	if target != expected || !leaseOwned(root, id) {
		return fmt.Errorf("runtime launch identity/lease changed")
	}
	start, e := ProcessStart(os.Getpid())
	if e != nil {
		return e
	}
	tx.New = Health{Build: Build{PID: os.Getpid()}, Start: start, Binary: target}
	if e = core.WriteJSON(runtimeTxPath(root), tx); e != nil {
		return e
	}
	return syscall.Exec(target, []string{target, "--state", root, "_project"}, os.Environ())
}
func startRuntime(root, launcher, target, id string) error {
	log, e := os.OpenFile(filepath.Join(root, "runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	c := exec.Command(launcher, "_update-runtime-start", root, id, target)
	c.Stdout, c.Stderr = log, log
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = c.Start(); e != nil {
		return e
	}
	return c.Process.Release()
}
func waitHealth(ctx context.Context, root, revision string) (Health, error) {
	for {
		if h, e := Call(ctx, root, "health", ""); e == nil && h.Build.Revision == revision && h.Build.Protocol == Protocol {
			return h, nil
		}
		select {
		case <-ctx.Done():
			return Health{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func stopRuntime(ctx context.Context, h Health) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	start, e := ProcessStart(h.Build.PID)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if start != h.Start {
		return RecoveryBlocked("process_identity_changed", "runtime PID was reused")
	}
	if h.Build.PID <= 1 {
		return RecoveryBlocked("container_init", "cannot replace container init process")
	}
	if e = syscall.Kill(h.Build.PID, syscall.SIGTERM); e != nil && e != syscall.ESRCH {
		return e
	}
	for {
		start, e = ProcessStart(h.Build.PID)
		if os.IsNotExist(e) || e == nil && start != h.Start {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func ReplaceRuntime(ctx context.Context, root, target string, r Release) error {
	return replaceRuntime(ctx, root, target, r, false)
}

// RecoverRuntime can repair an interrupted transaction without a responding
// runtime. It never creates a new update from the currently published release.
func RecoverRuntime(ctx context.Context, root string) error {
	return replaceRuntime(ctx, root, "", Release{}, true)
}

func replaceRuntime(ctx context.Context, root, target string, r Release, recoveryOnly bool) error {
	lock, e := core.Lock(filepath.Join(root, "runtime-update.lock"))
	if e != nil {
		return RecoveryWaiting("helper_running", e.Error())
	}
	defer lock.Close()
	tx, e := readRuntimeTransaction(root)
	if e != nil && !os.IsNotExist(e) {
		return RecoveryBlocked("transaction_unreadable", fmt.Sprintf("runtime transaction unreadable: %v", e))
	}
	if tx.Pending() {
		// Recovery finishes the captured transaction, even if upstream has moved.
		// Older transactions predate the embedded release; their original
		// manifest was written before the helper was invoked.
		if tx.Release != nil {
			r = *tx.Release
		} else if r.Revision != tx.Revision || target != tx.Target {
			b, err := os.ReadFile(filepath.Join("/cxz/tools/cxz-builds/releases", tx.Revision, "manifest.json"))
			if err != nil {
				return RecoveryBlocked("manifest_unavailable", fmt.Sprintf("runtime recovery manifest unavailable: %v", err))
			}
			if err = json.Unmarshal(b, &r); err != nil {
				return RecoveryBlocked("manifest_invalid", fmt.Sprintf("runtime recovery manifest invalid: %v", err))
			}
		}
		if r.Revision != tx.Revision {
			return RecoveryBlocked("release_identity_changed", "runtime recovery release identity changed")
		}
		target = tx.Target
	} else if recoveryOnly {
		return RecoveryBlocked("bootstrap_required", "no pending runtime update; runtime bootstrap required")
	}
	if e = CheckBinary(ctx, target, r, Current().Platform); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return RecoveryBlocked("candidate_invalid", fmt.Sprintf("runtime candidate verification failed: %v", e))
	}
	if tx.Pending() && tx.Release == nil {
		tx.Release = &r
		if e = core.WriteJSON(runtimeTxPath(root), tx); e != nil {
			return e
		}
	}
	launcher, e := os.Executable()
	if e != nil {
		return e
	}
	if (tx.State == "healthy" || tx.State == "rolled_back") && leaseOwned(root, tx.ID) {
		_, _ = Call(ctx, root, "release", tx.ID)
	}
	if tx.State == "healthy" && tx.Revision == r.Revision {
		return nil
	}
	if tx.State == "rolled_back" && tx.Revision == r.Revision && time.Since(tx.FailedAt) < Interval {
		return fmt.Errorf("previous runtime update rolled back: %s", tx.Error)
	}
	save := func(state string) error { tx.State = state; return core.WriteJSON(runtimeTxPath(root), tx) }
	if tx.ID == "" || tx.State == "healthy" || tx.State == "rolled_back" {
		id := core.ID()
		h, e := Call(ctx, root, "prepare", id)
		if e != nil {
			return e
		}
		if h.Build.Revision == r.Revision {
			_, e = Call(ctx, root, "release", id)
			return e
		}
		if !h.Build.Managed() || h.Build.Protocol != Protocol || h.Build.Schema != Schema {
			_, _ = Call(ctx, root, "release", id)
			return fmt.Errorf("runtime bootstrap required")
		}
		h.Runtime = nil
		tx = RuntimeTransaction{ID: id, Target: target, Revision: r.Revision, Release: &r, Old: h}
		if e = save("prepared"); e != nil {
			_, _ = Call(ctx, root, "release", id)
			return e
		}
	}
	if tx.Target != target || tx.Revision != r.Revision {
		return fmt.Errorf("another runtime update needs recovery")
	}
	if tx.State == "prepared" {
		// Recheck idle and renew admission after an interrupted helper.
		h, err := Call(ctx, root, "prepare", tx.ID)
		if err == nil {
			if h.Build.PID != tx.Old.Build.PID || h.Start != tx.Old.Start || h.Build.Revision != tx.Old.Build.Revision || h.Binary != tx.Old.Binary {
				_, _ = Call(ctx, root, "release", tx.ID)
				return RecoveryBlocked("process_identity_changed", "runtime identity changed")
			}
			// The previous lease may have expired while this helper was down.
			// Fresh admission and idle proof make the current runs the baseline.
			h.Runtime = nil
			tx.Old = h
			if e = save("prepared"); e != nil {
				_, _ = Call(ctx, root, "release", tx.ID)
				return e
			}
		} else {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			start, processErr := ProcessStart(tx.Old.Build.PID)
			if processErr == nil {
				if start != tx.Old.Start {
					return RecoveryBlocked("process_identity_changed", "runtime PID was reused")
				}
				return fmt.Errorf("runtime readiness unavailable: %w", err)
			}
			if !os.IsNotExist(processErr) {
				return processErr
			}
			// Losing the control plane is not permission to install a new
			// version while its agents may still be working. Restore the known
			// runtime first; a later forward update must obtain fresh admission.
			if !leaseOwned(root, tx.ID) {
				return RecoveryBlocked("lease_expired", "runtime lease expired; operator recovery required")
			}
			tx.Error = "runtime disappeared before readiness was confirmed; restoring previous runtime"
			tx.New = Health{}
			if e = save("rolling_back"); e != nil {
				return e
			}
		}
	}
	if tx.State == "prepared" {
		if !leaseOwned(root, tx.ID) {
			return RecoveryBlocked("lease_expired", "runtime lease expired; operator recovery required")
		}
		stop, cancel := context.WithTimeout(ctx, 30*time.Second)
		e = stopRuntime(stop, tx.Old)
		cancel()
		if e != nil {
			return e
		}
		if e = save("starting"); e != nil {
			return e
		}
		e = startRuntime(root, launcher, target, tx.ID)
		if e != nil {
			tx.Error = e.Error()
			tx.New = Health{}
			if e = save("rolling_back"); e != nil {
				return e
			}
		}
	}
	if tx.State == "starting" {
		wait, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, e = waitHealth(wait, root, r.Revision)
		cancel()
		// A caller stopping the helper is not evidence that the candidate
		// failed. Leave its recorded identity for the next recovery attempt.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Fence the child's durable publication while transitioning phases. A
		// delayed child must not start after rollback has already been selected.
		childLock, lockErr := core.Lock(filepath.Join(root, "runtime-start.lock"))
		if lockErr != nil {
			return lockErr
		}
		// The child publishes identity, so always reload before writing the next phase.
		latest, err := readRuntimeTransaction(root)
		if err != nil {
			childLock.Close()
			return err
		}
		tx = latest
		if e == nil {
			e = finishRuntime(ctx, root, &tx)
		}
		if e == nil {
			childLock.Close()
			return nil
		}
		tx.Error = e.Error()
		e = save("rolling_back")
		childLock.Close()
		if e != nil {
			return e
		}
	}
	if tx.State != "rolling_back" {
		return RecoveryBlocked("invalid_transaction", "invalid runtime transaction state")
	}
	finished, e := prepareRuntimeRollback(ctx, root, launcher, &tx)
	if e != nil || finished {
		return e
	}
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	_, e = waitHealth(wait, root, tx.Old.Build.Revision)
	cancel()
	if e != nil {
		return e
	}
	// The rollback child publishes its PID before serving health.
	tx, e = readRuntimeTransaction(root)
	if e != nil {
		return e
	}
	return finishRuntimeRollback(ctx, root, &tx)
}

// Serialize rollback decisions with child publication. An interrupted helper
// must neither overwrite a newly published PID nor spawn a second runtime.
func prepareRuntimeRollback(ctx context.Context, root, launcher string, tx *RuntimeTransaction) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	lock, e := core.Lock(filepath.Join(root, "runtime-start.lock"))
	if e != nil {
		return false, e
	}
	defer lock.Close()
	latest, e := readRuntimeTransaction(root)
	if e != nil {
		return false, e
	}
	if latest.ID != tx.ID || latest.State != "rolling_back" {
		return false, RecoveryBlocked("transaction_changed", "runtime rollback transaction changed")
	}
	*tx = latest
	if !leaseOwned(root, tx.ID) {
		return false, RecoveryBlocked("lease_expired", "runtime lease expired; refusing automatic rollback")
	}
	if h, err := Call(ctx, root, "health", ""); err == nil {
		if h.Lease != tx.ID {
			return false, RecoveryBlocked("foreign_lease", "runtime lease changed")
		}
		if h.Build.Revision == tx.Old.Build.Revision {
			return true, finishRuntimeRollback(ctx, root, tx)
		}
		if h.Build.PID != tx.New.Build.PID || h.Start != tx.New.Start {
			return false, RecoveryBlocked("process_identity_changed", "runtime process changed")
		}
	}
	if tx.New.Binary == tx.Old.Binary && tx.New.Build.PID != 0 {
		start, err := ProcessStart(tx.New.Build.PID)
		if err == nil {
			if start != tx.New.Start {
				return false, RecoveryBlocked("process_identity_changed", "runtime PID was reused")
			}
			return false, nil // The recorded rollback child is still starting.
		}
		if !os.IsNotExist(err) {
			return false, err
		}
	}
	if tx.New.Build.PID != 0 {
		if tx.New.Binary != tx.Old.Binary {
			stop, cancel := context.WithTimeout(ctx, 30*time.Second)
			e = stopRuntime(stop, tx.New)
			cancel()
			if e != nil {
				return false, e
			}
		}
		tx.New = Health{}
		if e = core.WriteJSON(runtimeTxPath(root), tx); e != nil {
			return false, e
		}
	}
	if e = CheckSupervisor(ctx, root, tx.Old.Binary); e != nil {
		return false, RecoveryBlocked("rollback_binary_unavailable", fmt.Sprintf("rollback binary unavailable: %v", e))
	}
	return false, startRuntime(root, launcher, tx.Old.Binary, tx.ID)
}
func finishRuntimeRollback(ctx context.Context, root string, tx *RuntimeTransaction) error {
	h, e := Call(ctx, root, "status", "")
	if e != nil {
		return e
	}
	expected := tx.Old
	if tx.New.Binary == tx.Old.Binary {
		expected = tx.New
	}
	if h.Build.Revision != tx.Old.Build.Revision || h.Build.PID != expected.Build.PID || h.Start != expected.Start || h.Lease != tx.ID || !reflect.DeepEqual(h.Sessions, tx.Old.Sessions) {
		return RecoveryBlocked("session_identity_changed", "rollback session identity mismatch")
	}
	tx.State = "rolled_back"
	tx.FailedAt = time.Now()
	if e = core.WriteJSON(runtimeTxPath(root), tx); e != nil {
		return e
	}
	_, e = Call(ctx, root, "release", tx.ID)
	return e
}
func finishRuntime(ctx context.Context, root string, tx *RuntimeTransaction) error {
	h, e := Call(ctx, root, "status", "")
	if e != nil {
		return e
	}
	if h.Build.Revision != tx.Revision || h.Build.PID != tx.New.Build.PID || h.Start != tx.New.Start || h.Lease != tx.ID || !reflect.DeepEqual(h.Sessions, tx.Old.Sessions) {
		return RecoveryBlocked("session_identity_changed", "runtime update changed live session identity")
	}
	tx.State = "healthy"
	if e = core.WriteJSON(runtimeTxPath(root), tx); e != nil {
		return e
	}
	_, e = Call(ctx, root, "release", tx.ID)
	return e
}
