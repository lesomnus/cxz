package versionuse

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/versionpin"
	"github.com/lesomnus/cxz/internal/workspace"
	"google.golang.org/grpc/metadata"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Process struct {
	PID   int
	Start string
	Args  []string
}
type Session struct {
	ID      string
	Process Process
	Resumed bool
}
type ProjectState struct {
	Generation, Version, Phase string
	Runtime                    []Process
	Sessions                   []Session
}

func process(pid int) (Process, error) {
	p := Process{PID: pid}
	b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if e != nil {
		return p, e
	}
	end := strings.LastIndexByte(string(b), ')')
	if end < 0 {
		return p, fmt.Errorf("invalid process stat")
	}
	f := strings.Fields(string(b[end+1:]))
	if len(f) < 20 {
		return p, fmt.Errorf("invalid process stat")
	}
	if f[0] == "Z" {
		return p, os.ErrNotExist
	}
	p.Start = f[19]
	b, e = os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if e != nil {
		return p, e
	}
	p.Args = strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
	return p, nil
}
func matches(args []string, root, role string) bool {
	if len(args) < 4 || args[1] != "--state" || args[2] != root || args[3] != role {
		return false
	}
	return (role == "_project" && len(args) == 4) || (role == "_supervise" && len(args) == 5)
}
func processes(root, role string) ([]Process, error) {
	entries, e := os.ReadDir("/proc")
	if e != nil {
		return nil, e
	}
	var out []Process
	for _, entry := range entries {
		pid, e := strconv.Atoi(entry.Name())
		if e != nil {
			continue
		}
		p, e := process(pid)
		if e != nil {
			if os.IsNotExist(e) || os.IsPermission(e) {
				continue
			}
			return nil, e
		}
		if matches(p.Args, root, role) {
			out = append(out, p)
		}
	}
	return out, nil
}
func stop(ctx context.Context, p Process) error {
	current, e := process(p.PID)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if p.PID <= 1 || current.Start != p.Start || strings.Join(current.Args, "\x00") != strings.Join(p.Args, "\x00") {
		return fmt.Errorf("process identity changed; refusing to signal PID %d", p.PID)
	}
	if e = syscall.Kill(p.PID, syscall.SIGTERM); e != nil && e != syscall.ESRCH {
		return e
	}
	force := time.Now().Add(10 * time.Second)
	for {
		now, e := process(p.PID)
		if os.IsNotExist(e) || e == nil && now.Start != p.Start {
			return nil
		}
		if e != nil {
			return e
		}
		if time.Now().After(force) {
			if e = syscall.Kill(p.PID, syscall.SIGKILL); e != nil && e != syscall.ESRCH {
				return e
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func projectState(root string) (ProjectState, error) {
	var s ProjectState
	b, e := os.ReadFile(filepath.Join(root, "use-project.json"))
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	return s, e
}
func saveProject(root string, s ProjectState) error {
	return core.WriteJSON(filepath.Join(root, "use-project.json"), s)
}
func fence(root, generation, phase string) error {
	return core.WriteJSON(filepath.Join(root, "use-barrier.json"), versionpin.Barrier{Generation: generation, Phase: phase})
}
func StopProject(ctx context.Context, root, generation, version string) error {
	if e := versionpin.Validate(version); e != nil {
		return e
	}
	lock, e := core.Lock(filepath.Join(root, "use-project.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	s, e := projectState(root)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if s.Generation != generation {
		if s.Generation != "" && s.Phase != "complete" {
			return fmt.Errorf("unfinished version switch %s; retry its version", s.Version)
		}
		s = ProjectState{Generation: generation, Version: version, Phase: "fencing"}
		if e = saveProject(root, s); e != nil {
			return e
		}
	}
	if s.Version != version {
		return fmt.Errorf("project transaction version mismatch")
	}
	if s.Phase == "stopped" || s.Phase == "resuming" || s.Phase == "complete" {
		return nil
	}
	if e = fence(root, generation, "stopping"); e != nil {
		return e
	}
	runtimes, e := processes(root, "_project")
	if e != nil {
		return e
	}
	s.Runtime = runtimes
	if e = saveProject(root, s); e != nil {
		return e
	}
	for _, p := range runtimes {
		if e = stop(ctx, p); e != nil {
			return e
		}
	}
	// No runtime can now start another session or admit a new input.
	runtimeLock, e := core.Lock(filepath.Join(root, "daemon.lock"))
	if e != nil {
		return fmt.Errorf("runtime is still serving: %w", e)
	}
	defer runtimeLock.Close()
	supervisors, e := processes(root, "_supervise")
	if e != nil {
		return e
	}
	for _, p := range supervisors {
		id := p.Args[4]
		if len(id) != 24 || strings.ContainsAny(id, "/\\.") {
			return fmt.Errorf("invalid supervisor session")
		}
		found := false
		for _, v := range s.Sessions {
			if v.ID == id {
				found = true
				if v.Process.Start != p.Start {
					return fmt.Errorf("session changed during interrupted version switch")
				}
			}
		}
		if !found {
			s.Sessions = append(s.Sessions, Session{ID: id, Process: p})
		}
	}
	if e = saveProject(root, s); e != nil {
		return e
	}
	for _, v := range s.Sessions {
		if e = stop(ctx, v.Process); e != nil {
			return e
		}
		if e = waitSession(ctx, root, v.ID); e != nil {
			return e
		}
	}
	for _, name := range []string{"run/update-lease.json", "runtime-update.json"} {
		if e = archive(root, name, generation); e != nil {
			return e
		}
	}
	dirs, e := os.ReadDir(filepath.Join(root, "sessions"))
	if e != nil {
		return e
	}
	for _, dir := range dirs {
		if dir.IsDir() {
			if e = archive(root, filepath.Join("sessions", dir.Name(), "agent-update.json"), generation); e != nil {
				return e
			}
		}
	}
	s.Phase = "stopped"
	return saveProject(root, s)
}
func archive(root, name, generation string) error {
	from := filepath.Join(root, name)
	e := os.Rename(from, from+".before-use-"+generation)
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func waitSession(ctx context.Context, root, id string) error {
	b, e := os.ReadFile(filepath.Join(core.Dir(root, id), "session.json"))
	if e != nil {
		return e
	}
	var m core.Session
	if e = json.Unmarshal(b, &m); e != nil {
		return e
	}
	for {
		var held []*os.File
		ready := true
		for _, path := range []string{filepath.Join(core.Dir(root, id), "supervisor.lock"), filepath.Join(accounts.Dir(accounts.SessionRoot(root, m.CreateID), m.Account), "login.lock")} {
			f, e := core.Lock(path)
			if e != nil {
				ready = false
				break
			}
			held = append(held, f)
		}
		for _, f := range held {
			f.Close()
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func ResumeProject(ctx context.Context, root, generation, version string) error {
	lock, e := core.Lock(filepath.Join(root, "use-project.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	s, e := projectState(root)
	if e != nil {
		return e
	}
	if s.Generation != generation || s.Version != version {
		return fmt.Errorf("project transaction mismatch")
	}
	if s.Phase == "complete" {
		e := os.Remove(filepath.Join(root, "use-barrier.json"))
		if os.IsNotExist(e) {
			return nil
		}
		return e
	}
	if s.Phase != "stopped" && s.Phase != "resuming" {
		return fmt.Errorf("project has not finished stopping")
	}
	if e = fence(root, generation, "resuming"); e != nil {
		return e
	}
	s.Phase = "resuming"
	if e = saveProject(root, s); e != nil {
		return e
	}
	if e = workspace.Boot(root); e != nil {
		return e
	}
	conn, e := transport.Dial(root)
	if e != nil {
		return e
	}
	defer conn.Close()
	client := resourceclient.New(conn)
	for {
		q, cancel := context.WithTimeout(ctx, time.Second)
		_, e = client.List(q, &api.Empty{})
		cancel()
		if e == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	for i, v := range s.Sessions {
		if v.Resumed {
			continue
		}
		current, e := client.Get(ctx, &api.SessionRef{Id: v.ID})
		if e != nil {
			return e
		}
		q := metadata.AppendToOutgoingContext(ctx, "cxz-use-id", generation)
		// Resume is idempotent for a live run. Never replay its last user input.
		result, e := client.Resume(q, &api.Control{SessionId: v.ID, RunId: current.RunId})
		if e != nil {
			return e
		}
		if result.State != "idle" && result.State != "working" && result.State != "waiting_input" {
			return fmt.Errorf("session %s did not initialize (%s)", v.ID, result.State)
		}
		s.Sessions[i].Resumed = true
		if e = saveProject(root, s); e != nil {
			return e
		}
	}
	s.Phase = "complete"
	if e = saveProject(root, s); e != nil {
		return e
	}
	return os.Remove(filepath.Join(root, "use-barrier.json"))
}
