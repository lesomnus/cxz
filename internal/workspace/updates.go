package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/dockerx"
)

type updateJob struct {
	Project, Session, Provider, Version, State, Reason string
	At                                                 time.Time
}
type updateState struct {
	CheckedAt time.Time
	Versions  map[string]string
	Errors    map[string]string
	Jobs      map[string]updateJob
	Failed    map[string]time.Time
}

func newerVersion(want, old string) bool {
	if !distribution.ValidVersion(want) || !distribution.ValidVersion(old) {
		return false
	}
	a, b := strings.Split(want, "."), strings.Split(old, ".")
	for i := range a {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return false
}
func binaryVersion(kind, path string) string {
	p := strings.Split(strings.TrimPrefix(path, "/cxz/tools/"), "/")
	if !strings.HasPrefix(path, "/cxz/tools/") || len(p) < 4 || p[0] != kind {
		return ""
	}
	return p[1]
}

// One manager owns the queue. No worker pool: at most one agent restart is in
// progress across all projects. Downloads do not stop running processes.
func (m *Manager) RunUpdates(ctx context.Context) {
	if value := os.Getenv("CXZ_AUTO_UPDATE"); value == "0" || strings.EqualFold(value, "false") {
		return
	}
	var state updateState
	path := filepath.Join(m.Root, "updates.json")
	if b, e := os.ReadFile(path); e == nil {
		_ = json.Unmarshal(b, &state)
	}
	if state.Versions == nil {
		state.Versions = map[string]string{}
	}
	if state.Errors == nil {
		state.Errors = map[string]string{}
	}
	if state.Jobs == nil {
		state.Jobs = map[string]updateJob{}
	}
	if state.Failed == nil {
		state.Failed = map[string]time.Time{}
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if time.Since(state.CheckedAt) >= 24*time.Hour {
			state.CheckedAt = time.Now()
			for _, kind := range []string{"claude", "codex", "gh"} {
				v, e := distribution.Latest(ctx, kind)
				if e != nil {
					state.Errors[kind] = e.Error()
					continue
				}
				delete(state.Errors, kind)
				state.Versions[kind] = v
			}
			if e := core.WriteJSON("/cxz/tools/releases.json", state.Versions); e != nil {
				state.Errors["publish"] = e.Error()
			} else {
				delete(state.Errors, "publish")
			}
		}
		m.rolloutOne(ctx, &state)
		if e := core.WriteJSON(path, state); e != nil {
			fmt.Fprintln(os.Stderr, "automatic updates: cannot persist queue:", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) rolloutOne(ctx context.Context, state *updateState) {
	projects, e := m.all(ctx)
	if e != nil {
		return
	}
	for _, p := range projects {
		lock := m.projectLock(p.ID)
		if !lock.TryLock() {
			continue
		}
		applied := func() bool {
			defer lock.Unlock()
			current, e := m.resolve(ctx, p.ID)
			if e != nil {
				return false
			}
			p = current
			work, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()
			conn, c, e := m.client(work, p)
			if e != nil {
				return false
			}
			defer conn.Close()
			list, e := c.List(work, &api.Empty{})
			if e != nil {
				return false
			}
			platform, e := dockerx.Run(work, "exec", p.ContainerID, "uname", "-m")
			if e != nil {
				return false
			}
			arch := strings.TrimSpace(string(platform))
			libc, e := dockerx.Run(work, "exec", p.ContainerID, "sh", "-c", "ls /lib/ld-musl-*.so.1 2>/dev/null || true")
			if e != nil {
				return false
			}
			allIdle := len(list.Sessions) > 0
			for _, s := range list.Sessions {
				if s.State == "stopped" || s.State == "failed" {
					continue
				}
				probe, e := c.UpdateAgent(work, &api.AgentUpdateInput{SessionId: s.Id, RunId: s.RunId})
				if e != nil {
					allIdle = false
					key := s.Id + "/" + state.Versions[s.Agent]
					state.Jobs[key] = updateJob{Project: p.ID, Session: s.Id, Provider: s.Agent, Version: state.Versions[s.Agent], State: "skipped", Reason: "readiness unavailable; update runtime/supervisor first", At: time.Now()}
					continue
				}
				if !probe.Ready {
					allIdle = false
				}
				version := state.Versions[s.Agent]
				if version == binaryVersion(s.Agent, probe.Binary) {
					key := s.Id + "/" + version
					if job, ok := state.Jobs[key]; ok && job.State != "updated" {
						job.State = "updated"
						job.Reason = "replacement observed running"
						job.At = time.Now()
						state.Jobs[key] = job
						delete(state.Failed, key)
					}
				}
				if !newerVersion(version, binaryVersion(s.Agent, probe.Binary)) {
					continue
				}
				key := s.Id + "/" + version
				job := updateJob{Project: p.ID, Session: s.Id, Provider: s.Agent, Version: version, State: "waiting", Reason: probe.Reason, At: time.Now()}
				if t := state.Failed[key]; !t.IsZero() && time.Since(t) < 24*time.Hour {
					continue
				}
				bin, e := distribution.EnsureVersion(work, "/cxz/tools", s.Agent, arch, len(strings.TrimSpace(string(libc))) > 0, version)
				if e == nil {
					_, e = dockerx.Run(work, "exec", "--user", p.RemoteUser, p.ContainerID, bin, "--version")
				}
				if e != nil {
					job.State = "download_failed"
					job.Reason = e.Error()
					state.Jobs[key] = job
					state.Failed[key] = time.Now()
					continue
				}
				probe, e = c.UpdateAgent(work, &api.AgentUpdateInput{SessionId: s.Id, RunId: s.RunId, Binary: bin})
				if e != nil {
					job.Reason = "readiness unavailable"
					state.Jobs[key] = job
					continue
				}
				job.Reason = probe.Reason
				state.Jobs[key] = job
				if !probe.Ready {
					continue
				}
				job.State = "applying"
				state.Jobs[key] = job
				// Persist intent before the external restart; the runtime also has
				// a per-session durable rollback record.
				if core.WriteJSON(filepath.Join(m.Root, "updates.json"), state) != nil {
					return false
				}
				result, e := c.UpdateAgent(work, &api.AgentUpdateInput{SessionId: s.Id, RunId: s.RunId, Binary: bin, Apply: true})
				if e != nil {
					job.State = "failed"
					job.Reason = e.Error()
					state.Failed[key] = time.Now()
				} else {
					job.State = result.State
					job.Reason = result.Reason
					if !result.Ready && result.State != "rolled_back" {
						job.State = "waiting"
					}
					if result.State == "rolled_back" {
						state.Failed[key] = time.Now()
					}
				}
				job.At = time.Now()
				state.Jobs[key] = job
				return true
			}
			// gh replacement is atomic and never restarts an agent. Only cxz's
			// own wrapper is updated, not an image-provided gh installation.
			if allIdle && distribution.ValidVersion(state.Versions["gh"]) {
				key := p.ID + "/gh/" + state.Versions["gh"]
				if state.Jobs[key].State == "updated" || (!state.Failed[key].IsZero() && time.Since(state.Failed[key]) < 24*time.Hour) {
					return false
				}
				old, err := dockerx.Run(work, "exec", p.ContainerID, "readlink", "/cxz/state/gh-bin")
				if err != nil || !newerVersion(state.Versions["gh"], binaryVersion("gh", strings.TrimSpace(string(old)))) {
					return false
				}
				bin, e := distribution.EnsureGitHubVersion(work, "/cxz/tools", arch, state.Versions["gh"])
				job := updateJob{Project: p.ID, Provider: "gh", Version: state.Versions["gh"], State: "updated", At: time.Now()}
				if e == nil {
					e = m.replaceGitHub(work, p, bin)
				}
				if e != nil {
					job.State = "skipped"
					job.Reason = e.Error()
					state.Failed[key] = time.Now()
				}
				state.Jobs[key] = job
				return true
			}
			return false
		}()
		if applied {
			return
		}
	}
}
