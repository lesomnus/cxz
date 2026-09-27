package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/selfupdate"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// Keep one terminal-owning launcher across successive updates. Children request
// their next restart after Bubble Tea cleanup instead of nesting more launchers.
func restartFrontend(r *cxzupdate.Restart) error {
	if os.Getenv("CXZ_FRONTEND_SLOT") != "" {
		return core.WriteJSON(cxzupdate.FrontendFile(r.Client.Root, "request"), r)
	}
	slot := fmt.Sprint(os.Getpid())
	env := append(os.Environ(), "CXZ_FRONTEND_SLOT="+slot)
	if e := os.Setenv("CXZ_FRONTEND_SLOT", slot); e != nil {
		return e
	}
	defer os.Unsetenv("CXZ_FRONTEND_SLOT")
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	for {
		next, e := runFrontendReplacement(r, env, signals)
		if e != nil || next == nil {
			return e
		}
		r = next
	}
}
func runFrontendReplacement(r *cxzupdate.Restart, env []string, signals <-chan os.Signal) (*cxzupdate.Restart, error) {
	lock, e := core.Lock(filepath.Join(r.Client.Root, "self-update.lock"))
	if e != nil {
		return nil, e
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e = cxzupdate.CheckBinary(ctx, r.Candidate, r.Release, cxzupdate.Current().Platform); e != nil {
		return nil, e
	}
	replacement, e := selfupdate.Prepare(r.Client.Executable)
	if e != nil {
		return nil, e
	}
	var previousBuild cxzupdate.Build
	info, err := exec.CommandContext(ctx, replacement.Target, "_build-info").Output()
	if err != nil || json.Unmarshal(info, &previousBuild) != nil {
		replacement.Close()
		return nil, fmt.Errorf("cannot identify installed frontend before replacement")
	}
	if !r.Release.CanReplace(previousBuild) {
		replacement.Close()
		return nil, fmt.Errorf("installed frontend advanced to another revision; restart cxz to use it")
	}
	// Another TUI may already have installed this exact image. Preserve its backup.
	changed := cxzupdate.Verify(replacement.Target, r.Release.Assets[cxzupdate.Current().Platform].SHA256) != nil
	if changed {
		if runtime.GOOS == "windows" {
			replacement.Previous = replacement.Target + ".previous-" + core.ID() + ".exe"
		}
		e = replacement.Stage(r.Candidate)
		if e == nil {
			_, e = replacement.Apply()
		}
	}
	if e != nil {
		replacement.Close()
		return nil, e
	}
	previous := replacement.Previous
	ready := cxzupdate.FrontendFile(r.Client.Root, "ready")
	request := cxzupdate.FrontendFile(r.Client.Root, "request")
	_ = os.Remove(ready)
	_ = os.Remove(request)
	defer os.Remove(ready)
	defer os.Remove(request)
	if e = core.WriteJSON(cxzupdate.ResumePath(r.Client.Root), r.Resume); e != nil {
		replacement.Close()
		return nil, e
	}
	state, _ := cxzupdate.Load(r.Client.Root)
	state.State = "restarting"
	state.Reason = ""
	_ = cxzupdate.Save(r.Client.Root, state)
	fmt.Fprintln(os.Stderr, "cxz: restarting frontend; agent sessions keep running")
	c := frontendCommand(r.Client.Executable, env)
	e = c.Start()
	acknowledged := false
	acknowledge := func() {
		if acknowledged {
			return
		}
		acknowledged = true
		latest, err := cxzupdate.Load(r.Client.Root)
		if err == nil {
			if latest.Release == nil || latest.Release.Revision == r.Release.Revision {
				latest.State = "healthy"
				latest.Reason = ""
			}
			latest.AppliedSequence = max(latest.AppliedSequence, r.Release.Sequence)
			latest.Running = cxzupdate.Build{Revision: r.Release.Revision, Platform: cxzupdate.Current().Platform, Protocol: r.Release.Protocol, Schema: r.Release.Schema, PID: c.Process.Pid}
			_ = cxzupdate.Save(r.Client.Root, latest)
		}
		replacement.Close()
		lock.Close()
	}
	if e == nil {
		done := make(chan error, 1)
		go func() { done <- c.Wait() }()
		timeout := time.NewTimer(45 * time.Second)
		defer timeout.Stop()
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
	wait:
		for {
			select {
			case sig := <-signals:
				// Terminal signals normally reach the whole group; also support direct signals.
				_ = c.Process.Signal(sig)
			case <-tick.C:
				if !acknowledged {
					if _, err := os.Stat(ready); err == nil {
						acknowledge()
						timeout.Stop()
					}
				}
			case e = <-done:
				// A short-lived but initialized TUI may exit before the polling tick.
				if _, err := os.Stat(ready); err == nil {
					acknowledge()
				}
				break wait
			case <-timeout.C:
				if _, err := os.Stat(ready); err == nil {
					acknowledge()
					continue
				}
				_ = c.Process.Kill()
				<-done
				e = fmt.Errorf("frontend startup timed out")
				break wait
			}
		}
	}
	if acknowledged {
		replacement.Close()
		b, err := os.ReadFile(request)
		if os.IsNotExist(err) {
			return nil, e
		}
		if err != nil {
			return nil, err
		}
		var next cxzupdate.Restart
		if err = json.Unmarshal(b, &next); err != nil {
			return nil, err
		}
		if next.Client != r.Client {
			return nil, fmt.Errorf("frontend restart identity changed")
		}
		return &next, nil
	}
	if e == nil {
		e = fmt.Errorf("frontend exited before initialization")
	}
	// Keep executable locking through startup and rollback. No other frontend can
	// overwrite the backup while the replacement is still uncommitted.
	state.State = "rolled_back"
	state.Reason = e.Error()
	state.FailedAt = time.Now()
	state.FailedRevision = r.Release.Revision
	_ = cxzupdate.Save(r.Client.Root, state)
	if !changed {
		replacement.Close()
		return nil, fmt.Errorf("installed frontend failed startup: %w", e)
	}
	if err := restoreFrontend(r.Client.Executable, previous); err != nil {
		replacement.Close()
		return nil, fmt.Errorf("frontend startup: %v; rollback: %w", e, err)
	}
	replacement.Close()
	lock.Close()
	// Restore safe navigation metadata; no prompt is saved or replayed.
	r.Resume.Revision = previousBuild.Revision
	_ = core.WriteJSON(cxzupdate.ResumePath(r.Client.Root), r.Resume)
	fmt.Fprintln(os.Stderr, "cxz: previous frontend restored")
	old := frontendCommand(r.Client.Executable, env)
	if e = old.Run(); e != nil {
		return nil, e
	}
	if b, err := os.ReadFile(request); err == nil {
		var next cxzupdate.Restart
		if err = json.Unmarshal(b, &next); err != nil {
			return nil, err
		}
		if next.Client != r.Client {
			return nil, fmt.Errorf("frontend restart identity changed")
		}
		return &next, nil
	}
	return nil, nil
}
func frontendCommand(path string, env []string) *exec.Cmd {
	c := exec.Command(path, os.Args[1:]...)
	c.Env = env
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c
}
