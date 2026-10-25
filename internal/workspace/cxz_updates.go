package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/networkservices"
	"strings"
)

func (m *Manager) projectHealth(ctx context.Context, p *Project, action, tx string) (cxzupdate.Health, error) {
	if _, e := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.storageID()); e != nil {
		return cxzupdate.Health{}, e
	}
	return cxzupdate.ContainerHealth(ctx, p.ContainerID, p.RemoteUser, "/cxz/state/data", action, tx)
}

// quietSweep bounds the whole sweep, and quietSweepProjects how many projects
// are asked at once. Each answer costs a container inspect and a round trip
// into that project's runtime, which asks every session it holds -- so asking
// them one after another made this grow with the installation. An owner with
// twenty-five sessions could not finish inside the helper's thirty seconds,
// which is cxz#204: the rollout retried forever and never updated anything.
const quietSweep = 20 * time.Second
const quietSweepProjects = 4

func (m *Manager) QuietForUpdate(ctx context.Context) (map[string]string, error) {
	ps, e := m.all(ctx)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, quietSweep)
	defer cancel()
	type answer struct {
		sessions map[string]string
		blocked  error
	}
	out := make([]answer, len(ps))
	gate := make(chan struct{}, quietSweepProjects)
	var wait sync.WaitGroup
	for i, p := range ps {
		if p.ContainerID == "" {
			continue
		}
		wait.Add(1)
		go func(i int, p *Project) {
			defer wait.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			if ctx.Err() != nil {
				out[i] = answer{blocked: fmt.Errorf("project %s: not asked before the sweep ran out of time", p.ID)}
				return
			}
			// One inspect, not two: projectHealth makes the same call, and it
			// is the call that says whether the container is still ours.
			v, e := dockerx.Owned(ctx, p.ContainerID, m.Owner, p.storageID())
			if e != nil {
				out[i] = answer{blocked: e}
				return
			}
			if !v.State.Running {
				return
			}
			h, e := cxzupdate.ContainerHealth(ctx, p.ContainerID, p.RemoteUser, "/cxz/state/data", "status", "")
			if e != nil {
				out[i] = answer{blocked: fmt.Errorf("project %s: bootstrap/readiness unavailable: %w", p.ID, e)}
				return
			}
			if !h.Ready {
				out[i] = answer{sessions: h.Sessions, blocked: fmt.Errorf("project %s: %s", p.ID, h.Reason)}
				return
			}
			out[i] = answer{sessions: h.Sessions}
		}(i, p)
	}
	wait.Wait()
	runs := map[string]string{}
	var blocked error
	for _, a := range out {
		for id, run := range a.sessions {
			runs[id] = run
		}
		if a.blocked != nil && blocked == nil {
			blocked = a.blocked
		}
	}
	return runs, blocked
}

// RunCxzUpdates serializes with provider updates and external manager helpers.
// The published manifest is pinned before any runtime or session mutation.
func (m *Manager) RunCxzUpdates(ctx context.Context) {
	if !cxzupdate.Current().Managed() {
		return
	}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		m.cxzUpdateStep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// note records where this tick stopped, for the paths that used to return
// without saying anything. A reason left over from an earlier tick is worse
// than none: it sends whoever is asking to a condition that has already
// cleared. Today that was an hour of chasing a lock nobody held, and it is the
// same cost cxz#222 paid for the same silence. Written only when it changes, so
// a paused installation does not rewrite the file every thirty seconds.
// See cxz#253.
func (m *Manager) noteUpdateStop(state, reason string) {
	s, err := cxzupdate.Load(m.Root)
	if err != nil || (s.State == state && s.Reason == reason) {
		return
	}
	s.State, s.Reason = state, reason
	_ = cxzupdate.Save(m.Root, s)
}

// footnoteUpdate says why a pass could not look without pretending the
// installation moved. A lock collision is not a state: this tick meeting the
// previous one, or a project whose lock a client holds for a moment, used to be
// written as "holding" and "waiting" -- and an installation with nothing left to
// do then read as waiting for twelve hours, which is the same lie cxz#222 and
// cxz#253 were about. See cxz#257.
func (m *Manager) footnoteUpdate(reason string) {
	s, err := cxzupdate.Load(m.Root)
	if err != nil || s.Reason == reason {
		return
	}
	s.Reason = reason
	_ = cxzupdate.Save(m.Root, s)
}

// tryLockFor waits a little for a lock instead of giving up on the first
// attempt. A client listing sessions holds a project's lock for a moment, and a
// pass that gives up instantly can lose the same race every time: harmless
// while everything is current, but a project that is actually behind would
// never be updated, and the installation would say a project operation was in
// progress while nothing was. See cxz#257.
func tryLockFor(mu *sync.Mutex, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if mu.TryLock() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (m *Manager) cxzUpdateStep(ctx context.Context) {
	// Policy belongs to this installation, not whichever frontend last connected.
	cfg, err := cxzupdate.Policy(m.Root)
	if err != nil {
		m.noteUpdateStop("waiting", "update policy unreadable: "+err.Error())
		return
	}
	if !cfg.Active() {
		m.noteUpdateStop("waiting", "automatic updates are paused")
		return
	}
	lock, e := core.Lock(filepath.Join(m.Root, "rollout.lock"))
	if e != nil {
		// Say so. A rollout that could not even look is not the same as one
		// that looked and found nothing, and this silence -- a status whose
		// timestamp simply stops moving -- cost an hour of looking in the
		// wrong places. See cxz#222.
		m.footnoteUpdate("a rollout holds the lock: " + e.Error())
		return
	}
	defer lock.Close()
	work, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Reconcile siblings against the actual running manager, including the first
	// tick after an old helper replaced it. This also retries independently of
	// release-channel availability and never opens the host install directory.
	var networkErr error
	if m.Container != "" && m.Owner != "" {
		report, err := networkservices.Refresh(work, m.Root, m.Container, m.Owner)
		networkErr = err
		state, loadErr := cxzupdate.Load(m.Root)
		if loadErr != nil {
			m.noteUpdateStop("waiting", "network service status unreadable: "+loadErr.Error())
			return
		}
		state.NetworkServices = &report
		if err = cxzupdate.Save(m.Root, state); err != nil {
			return
		}
	}
	s, e := cxzupdate.Check(work, m.Root, false)
	if e != nil {
		m.noteUpdateStop("waiting", "release channel unavailable: "+e.Error())
		return
	}
	if s.Release == nil {
		m.noteUpdateStop("waiting", "no release known for the selected channel")
		return
	}
	if !s.RetryAllowed() {
		m.noteUpdateStop("rolled_back", fmt.Sprintf("%s failed at %s; retrying after %s",
			s.FailedRevision[:min(12, len(s.FailedRevision))], s.FailedAt.UTC().Format(time.RFC3339), s.FailedAt.Add(cxzupdate.Interval).UTC().Format(time.RFC3339)))
		return
	}
	// A refusal is an answer: the server said it was not ready, and asking
	// again every tick re-runs the readiness sweep across every session for a
	// rollout that is about to be refused again. See cxz#210.
	if a, refused := cxzupdate.Refused(m.Root, time.Now()); refused {
		s.State = "waiting"
		s.Reason = a.Reason
		_ = cxzupdate.Save(m.Root, s)
		return
	}
	r := *s.Release
	if !r.CanReplace(cxzupdate.Current()) {
		s.State = "waiting"
		s.Reason = "selected channel cannot replace the running build"
		_ = cxzupdate.Save(m.Root, s)
		return
	}
	if r.Revision != cxzupdate.Current().Revision {
		// The independent helper acquires rollout.lock after this step returns.
		// Never forced on the tick: forcing is something a person asks for.
		e = cxzupdate.ScheduleManager(work, m.Root, m.Container, m.Owner, r, false)
		if e != nil {
			s.State = "waiting"
			s.Reason = e.Error()
		} else {
			s.State = "applying"
			s.Reason = "manager helper scheduled"
		}
		_ = cxzupdate.Save(m.Root, s)
		return
	}
	s.State = "healthy"
	s.Reason = ""
	ps, e := m.all(work)
	if e != nil {
		m.noteUpdateStop("waiting", "project inventory unavailable: "+e.Error())
		return
	}
	var busy []string
	for _, p := range ps {
		projectLock := m.projectLock(p.ID)
		if !tryLockFor(projectLock, 2*time.Second) {
			busy = append(busy, p.ID)
			continue
		}
		// Bound by the step context; prevent concurrent manual project recreation.
		defer projectLock.Unlock()
		current, err := m.resolve(work, p.ID)
		if err != nil {
			s.State = "waiting"
			s.Reason = err.Error()
			continue
		}
		p = current
		if p.ContainerID == "" {
			continue
		}
		v, err := dockerx.Owned(work, p.ContainerID, m.Owner, p.storageID())
		if err != nil {
			s.State = "waiting"
			s.Reason = err.Error()
			continue
		}
		if !v.State.Running {
			continue
		}
		h, err := m.projectHealth(work, p, "status", "")
		if err != nil {
			s.State = "waiting"
			s.Reason = "project runtime bootstrap required: " + p.ID
			continue
		}
		if !h.Build.Managed() || !r.CanReplace(h.Build) {
			s.State = "waiting"
			s.Reason = "project runtime is unmanaged or newer than edge: " + p.ID
			continue
		}
		recoverRuntime := h.Runtime != nil && h.Runtime.State != "healthy" && h.Runtime.State != "rolled_back"
		if h.Lease != "" && (!recoverRuntime || h.Lease != h.Runtime.ID) {
			s.State = "applying"
			s.Reason = "project maintenance: " + p.ID
			_ = cxzupdate.Save(m.Root, s)
			return
		}
		if h.Runtime != nil && h.Runtime.Revision == r.Revision && h.Runtime.State == "rolled_back" && time.Since(h.Runtime.FailedAt) < cxzupdate.Interval {
			s.State = "rolled_back"
			s.Reason = h.Runtime.Error
			s.FailedAt = h.Runtime.FailedAt
			s.FailedRevision = r.Revision
			_ = cxzupdate.Save(m.Root, s)
			return
		}
		binary, err := cxzupdate.Stage(work, "/cxz/tools/cxz-builds", r, h.Build.Platform)
		if cxzupdate.EdgeAssetMissing(r, err) {
			// Refresh before the next rollout tick. A newer manager must be scheduled
			// before any project runtime can move to the new release.
			_, _ = cxzupdate.Check(work, m.Root, true)
			m.noteUpdateStop("waiting", "release assets are not published yet: "+p.ID)
			return
		}
		if err == nil {
			var b []byte
			b, err = dockerx.Run(work, "exec", "--user", p.RemoteUser, p.ContainerID, binary, "_build-info")
			var build cxzupdate.Build
			if err == nil && (json.Unmarshal(b, &build) != nil || build.Revision != r.Revision || build.Platform != h.Build.Platform || build.Protocol != r.Protocol || build.Schema != r.Schema || !build.Managed()) {
				err = fmt.Errorf("candidate identity mismatch")
			}
		}
		if err != nil {
			s.State = "waiting"
			s.Reason = err.Error()
			continue
		}
		if h.Build.Revision != r.Revision || recoverRuntime {
			s.State = "waiting"
			s.Reason = "project " + p.ID + ": " + h.Reason
			if !h.Ready && !recoverRuntime {
				continue
			}
			manifest := filepath.Join("/cxz/tools/cxz-builds/releases", r.Revision, "manifest.json")
			if err = core.WriteJSON(manifest, r); err == nil {
				err = os.Chmod(manifest, 0644)
			}
			if err == nil {
				_, err = dockerx.Run(work, "exec", "--user", p.RemoteUser, p.ContainerID, binary, "_update-runtime", "/cxz/state/data", binary, manifest)
			}
			if err != nil {
				s.Reason = err.Error()
			} else {
				s.State = "applying"
				s.Reason = "project runtime update: " + p.ID
			}
			_ = cxzupdate.Save(m.Root, s)
			return
		}
		conn, client, err := m.client(work, p)
		if err != nil {
			s.State = "waiting"
			s.Reason = err.Error()
			continue
		}
		list, err := client.List(work, &api.Empty{})
		if err != nil {
			conn.Close()
			s.State = "waiting"
			s.Reason = err.Error()
			continue
		}
		applied := false
		for _, session := range list.Sessions {
			if session.State == "stopped" || session.State == "failed" {
				continue
			}
			status, err := client.UpdateAgent(work, &api.AgentUpdateInput{SessionId: session.Id, RunId: session.RunId})
			if err != nil {
				s.State = "waiting"
				s.Reason = "supervisor readiness unavailable"
				continue
			}
			if status.Revision == r.Revision {
				continue
			}
			s.State = "waiting"
			s.Reason = "session " + session.Id + ": " + status.Reason
			if status.Protocol != cxzupdate.Protocol || !r.CanReplace(cxzupdate.Build{Revision: status.Revision}) {
				s.Reason = "supervisor bootstrap required: " + session.Id
				continue
			}
			if !status.Ready {
				continue
			}
			agentBinary, err := m.combinedAgentTarget(work, p, session, status.Binary)
			if err != nil {
				s.Reason = err.Error()
				continue
			}
			result, err := client.UpdateAgent(work, &api.AgentUpdateInput{SessionId: session.Id, RunId: session.RunId, Binary: agentBinary, SupervisorBinary: binary, Apply: true})
			if err != nil {
				s.Reason = err.Error()
			} else if result.State == "rolled_back" {
				s.State = "rolled_back"
				s.Reason = result.Reason
				s.FailedAt = time.Now()
				s.FailedRevision = r.Revision
			} else if result.State == "updated" {
				s.State = "waiting"
				s.Reason = "session updated: " + session.Id
				if agentBinary != "" {
					if err := m.promoteAgentDefault(work, p, session.Agent, result); err != nil {
						s.Reason = "session updated; project agent default: " + err.Error()
					}
				}
			} else {
				s.State = "waiting"
				s.Reason = result.Reason
			}
			applied = true
			break
		}
		conn.Close()
		if applied {
			_ = cxzupdate.Save(m.Root, s)
			return
		}
	}
	if s.State == "healthy" && networkErr != nil {
		s.State = "waiting"
		s.Reason = "network services: " + networkErr.Error()
	}
	if s.State == "healthy" {
		s.AppliedSequence = r.Sequence
		// Everything this pass could reach is current. Projects it could not
		// reach are a footnote, not a pending update: naming one as the state
		// is how an installation with nothing to do came to report "project
		// operation in progress" for twelve hours. See cxz#257.
		if len(busy) > 0 {
			s.Reason = "busy this pass, will look again: " + strings.Join(busy, ", ")
		}
	}
	_ = cxzupdate.Save(m.Root, s)
}

// Both queues share the same stop transaction. A pending provider release is
// validated first, then applied with cxz in one idle restart.
func (m *Manager) combinedAgentTarget(ctx context.Context, p *Project, s *api.Session, current string) (string, error) {
	if v := os.Getenv("CXZ_AUTO_UPDATE"); v == "0" || strings.EqualFold(v, "false") {
		return "", nil
	}
	var state updateState
	b, e := os.ReadFile(filepath.Join(m.Root, "updates.json"))
	if os.IsNotExist(e) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	if e = json.Unmarshal(b, &state); e != nil {
		return "", e
	}
	version := state.Versions[s.Agent]
	if !newerVersion(version, binaryVersion(s.Agent, current)) {
		return "", nil
	}
	if at := state.Failed[s.Id+"/"+version]; !at.IsZero() && time.Since(at) < 24*time.Hour {
		return "", nil
	}
	arch, e := dockerx.Run(ctx, "exec", p.ContainerID, "uname", "-m")
	if e != nil {
		return "", e
	}
	libc, e := dockerx.Run(ctx, "exec", p.ContainerID, "sh", "-c", "ls /lib/ld-musl-*.so.1 2>/dev/null || true")
	if e != nil {
		return "", e
	}
	bin, e := distribution.EnsureVersion(ctx, "/cxz/tools", s.Agent, strings.TrimSpace(string(arch)), len(strings.TrimSpace(string(libc))) > 0, version)
	if e != nil {
		return "", e
	}
	_, e = dockerx.Run(ctx, "exec", "--user", p.RemoteUser, p.ContainerID, bin, "--version")
	return bin, e
}
