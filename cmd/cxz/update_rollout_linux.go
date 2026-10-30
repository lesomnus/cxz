package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/dockerx"
)

// rolloutDetail is the part of a stalled rollout that lives outside
// cxz-update.json: the manager transaction and the lock the helper takes. Both
// cost an afternoon to read by hand -- a staged transaction for a superseded
// release stops every rollout (cxz#251), and a status that says "holding" is
// indistinguishable from a leaked lock until you have gone looking in
// /proc/locks. Asking one command is the point. See cxz#253.
func rolloutDetail(ctx context.Context, root string) map[string]any {
	out := map[string]any{}
	if recoveries, err := cxzupdate.ReadRecoveries(root); err == nil {
		out["runtime_recoveries"] = recoveries
	} else {
		out["runtime_recovery_error"] = err.Error()
	}
	if tx, err := cxzupdate.ReadManagerTransaction(root); err == nil && tx.ID != "" {
		d := map[string]any{"id": tx.ID, "state": tx.State, "revision": tx.Release.Revision, "old_container": tx.OldID}
		if tx.Error != "" {
			d["error"] = tx.Error
		}
		// Whether the old manager is still there decides between recovery and a
		// rollback, so say it rather than leaving it to be guessed.
		_, e := dockerx.Inspect(ctx, tx.OldID)
		d["old_container_present"] = !dockerx.Missing(e)
		out["manager_transaction"] = d
	}
	held, err := lockHeld(filepath.Join(root, "rollout.lock"))
	switch {
	case err != nil:
		out["rollout_lock"] = "unknown: " + err.Error()
	case held:
		out["rollout_lock"] = "held"
	default:
		out["rollout_lock"] = "free"
	}
	return out
}

// applyRollout starts the manager rollout now rather than on the next thirty
// second tick, and with force the old manager hands over without the readiness
// sweep. Until this existed the only way past a session that never reports
// itself quiet was to stop that session, which is a strange thing to have to do
// to install an update. See cxz#253.
func applyRollout(ctx context.Context, root, container, owner string, force bool) ([]byte, error) {
	lock, e := core.Lock(filepath.Join(root, "rollout.lock"))
	if e != nil {
		return nil, fmt.Errorf("a rollout is already running: %w", e)
	}
	// Held across the scheduling call and no longer, exactly as the rollout tick
	// does it: the helper takes the lock after this returns.
	defer lock.Close()
	s, e := cxzupdate.Check(ctx, root, true)
	if e != nil {
		return nil, e
	}
	if s.Release == nil {
		return nil, fmt.Errorf("no release published for the selected channel")
	}
	r := *s.Release
	if r.Revision == cxzupdate.Current().Revision {
		return nil, fmt.Errorf("the manager already runs %s; project runtimes and sessions follow on the rollout tick", r.Revision[:12])
	}
	if !r.CanReplace(cxzupdate.Current()) {
		return nil, fmt.Errorf("the selected channel cannot replace the running build")
	}
	if e = cxzupdate.ScheduleManager(ctx, root, container, owner, r, force); e != nil {
		return nil, e
	}
	s.State, s.Reason = "applying", "manager helper scheduled"
	if force {
		s.Reason += " (forced)"
	}
	if e = cxzupdate.Save(root, s); e != nil {
		return nil, e
	}
	b, e := json.MarshalIndent(map[string]any{"scheduled": r.Revision, "forced": force, "update": s}, "", "  ")
	return append(b, '\n'), e
}

// lockHeld answers without taking the lock. Asking for it to find out would
// make the next rollout tick report a holder that is this very command.
func lockHeld(path string) (bool, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	f, err := os.Open("/proc/locks")
	if err != nil {
		return false, err
	}
	defer f.Close()
	want := fmt.Sprintf(":%d", st.Ino)
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		for _, field := range strings.Fields(scan.Text()) {
			// MAJ:MIN:INODE, and nothing else in the line carries two colons.
			if strings.Count(field, ":") == 2 && strings.HasSuffix(field, want) {
				return true, nil
			}
		}
	}
	return false, scan.Err()
}
