package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/dockerx"
)

func (m *Manager) runtimeRecoveryPath(p *Project) string {
	return filepath.Join(m.Root, "projects", p.storageID(), "runtime-recovery.json")
}

func (m *Manager) recoverRuntime(ctx context.Context, p *Project, release cxzupdate.Release, h cxzupdate.Health, available, inspected bool) (cxzupdate.Recovery, bool, error) {
	o := cxzupdate.RecoveryObservation{
		Project: p.ID, Name: p.Name, Container: p.ContainerID,
		Available: available, Idle: inspected && h.Ready, LeaseUntil: h.LeaseUntil,
		Finalize: cxzupdate.RuntimeFinalizationKey(h),
	}
	if h.Runtime != nil {
		o.Transaction, o.Phase = h.Runtime.ID, h.Runtime.State
		o.Waiting = inspected && h.Runtime.State == "prepared" && !h.Ready && h.Lease == "" && (h.Failure == nil || h.Failure.Class == "waiting")
		o.Reason = h.Reason
	}
	return cxzupdate.RunRecovery(ctx, m.runtimeRecoveryPath(p), o, time.Now, func(finalize bool) cxzupdate.RecoveryResult {
		result := cxzupdate.RecoveryResult{Transaction: o.Transaction, Phase: o.Phase, Available: available, LeaseUntil: o.LeaseUntil}
		platform := h.Build.Platform
		if platform == "" {
			platform = cxzupdate.Current().Platform
		}
		binary, err := m.runtimeUpdater(ctx, p, release, platform)
		if err != nil {
			if cxzupdate.EdgeAssetMissing(release, err) {
				_, _ = cxzupdate.Check(ctx, m.Root, true)
			}
			result.Failure = cxzupdate.RecoveryProblem(err)
			return result
		}
		action := "_recover-runtime"
		if finalize {
			action = "_finalize-runtime"
		}
		b, err := dockerx.Run(ctx, "exec", "--user", p.RemoteUser, p.ContainerID, binary, action, "/cxz/state/data", "--json")
		if err != nil {
			result.Failure = cxzupdate.RecoveryProblem(err)
			return result
		}
		var reported cxzupdate.RecoveryResult
		if err := json.Unmarshal(b, &reported); err != nil {
			result.Failure = cxzupdate.RecoveryProblem(fmt.Errorf("invalid runtime recovery report: %w", err))
			return result
		}
		if reported.Outcome != "completed" && reported.Outcome != "waiting" && reported.Outcome != "failed" && reported.Outcome != "stopped" || (reported.Outcome == "completed") != (reported.Failure == nil) {
			result.Failure = cxzupdate.RecoveryProblem(fmt.Errorf("invalid runtime recovery report outcome"))
			return result
		}
		return reported
	})
}

func recoveryReason(r cxzupdate.Recovery) string {
	text := fmt.Sprintf("project %s: runtime recovery %s (%d/%d attempts)", r.Project, r.Status, r.Attempts, cxzupdate.RecoveryAttempts)
	if r.Reason != "" {
		text += ": " + r.Reason
	}
	if !r.NextAttempt.IsZero() {
		text += "; next attempt " + r.NextAttempt.UTC().Format(time.RFC3339)
	}
	return text
}

// RetryRuntimeRecovery is called under rollout.lock by the installed Manager's
// host command. Resolve against project manifests, then reset only scheduling.
func RetryRuntimeRecovery(root, ref string) (cxzupdate.Recovery, error) {
	paths, err := filepath.Glob(filepath.Join(root, "projects", "*", "project.json"))
	if err != nil {
		return cxzupdate.Recovery{}, err
	}
	var named []*Project
	var exact *Project
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return cxzupdate.Recovery{}, err
		}
		var p Project
		if err := json.Unmarshal(b, &p); err != nil {
			return cxzupdate.Recovery{}, err
		}
		if p.storageID() != filepath.Base(filepath.Dir(path)) {
			return cxzupdate.Recovery{}, fmt.Errorf("project manifest identity mismatch")
		}
		if p.ID == ref || p.storageID() == ref || p.Workspace == ref {
			exact = &p
		} else if p.Name == ref {
			named = append(named, &p)
		}
	}
	if exact == nil {
		if len(named) > 1 {
			return cxzupdate.Recovery{}, fmt.Errorf("ambiguous project name; use its id or workspace path")
		}
		if len(named) == 1 {
			exact = named[0]
		}
	}
	if exact == nil {
		return cxzupdate.Recovery{}, fmt.Errorf("project not found: %s", ref)
	}
	m := &Manager{Root: root}
	return cxzupdate.ResetRecovery(m.runtimeRecoveryPath(exact))
}
