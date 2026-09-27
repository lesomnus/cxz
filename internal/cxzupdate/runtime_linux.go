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
	start, e := ProcessStart(h.Build.PID)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if start != h.Start {
		return fmt.Errorf("runtime PID was reused")
	}
	if h.Build.PID <= 1 {
		return fmt.Errorf("cannot replace container init process")
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
	lock, e := core.Lock(filepath.Join(root, "runtime-update.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = CheckBinary(ctx, target, r, Current().Platform); e != nil {
		return e
	}
	tx, e := readRuntimeTransaction(root)
	if e != nil && !os.IsNotExist(e) {
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
		tx = RuntimeTransaction{ID: id, Target: target, Revision: r.Revision, Old: h}
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
			if h.Build.PID != tx.Old.Build.PID || h.Start != tx.Old.Start || !reflect.DeepEqual(h.Sessions, tx.Old.Sessions) {
				return fmt.Errorf("runtime identity changed")
			}
		} else if start, err := ProcessStart(tx.Old.Build.PID); err == nil && start == tx.Old.Start {
			return fmt.Errorf("runtime readiness unavailable")
		}
		if !leaseOwned(root, tx.ID) {
			return fmt.Errorf("runtime lease expired; operator recovery required")
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
		e = startRuntime(root, target, target, tx.ID)
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
		return fmt.Errorf("invalid runtime transaction state")
	}
	if !leaseOwned(root, tx.ID) {
		return fmt.Errorf("runtime lease expired; refusing automatic rollback")
	}
	if h, err := Call(ctx, root, "health", ""); err == nil {
		if h.Lease != tx.ID {
			return fmt.Errorf("runtime lease changed")
		}
		if h.Build.Revision == tx.Old.Build.Revision {
			return finishRuntimeRollback(ctx, root, &tx)
		}
		if h.Build.PID != tx.New.Build.PID || h.Start != tx.New.Start {
			return fmt.Errorf("runtime process changed")
		}
	}
	// Do not spawn a duplicate rollback after its child has already been recorded.
	if tx.New.Binary != tx.Old.Binary {
		if tx.New.Build.PID != 0 {
			stop, cancel := context.WithTimeout(ctx, 30*time.Second)
			e = stopRuntime(stop, tx.New)
			cancel()
			if e != nil {
				return e
			}
		}
		tx.New = Health{}
		if e = save("rolling_back"); e != nil {
			return e
		}
	}
	if tx.New.Build.PID == 0 {
		if e = startRuntime(root, target, tx.Old.Binary, tx.ID); e != nil {
			return e
		}
	}
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	_, e = waitHealth(wait, root, tx.Old.Build.Revision)
	cancel()
	if e != nil {
		return e
	}
	return finishRuntimeRollback(ctx, root, &tx)
}
func finishRuntimeRollback(ctx context.Context, root string, tx *RuntimeTransaction) error {
	h, e := Call(ctx, root, "status", "")
	if e != nil {
		return e
	}
	if h.Build.Revision != tx.Old.Build.Revision || !reflect.DeepEqual(h.Sessions, tx.Old.Sessions) {
		return fmt.Errorf("rollback session identity mismatch")
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
		return fmt.Errorf("runtime update changed live session identity")
	}
	tx.State = "healthy"
	if e = core.WriteJSON(runtimeTxPath(root), tx); e != nil {
		return e
	}
	_, e = Call(ctx, root, "release", tx.ID)
	return e
}
