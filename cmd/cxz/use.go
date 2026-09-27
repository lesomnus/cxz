package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/selfupdate"
	"github.com/lesomnus/cxz/internal/versionpin"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type useBackend struct {
	pin    versionpin.Pin
	apply  func(context.Context) error
	finish func(context.Context) error
}

func useCommand() *xli.Command {
	selected := ""
	unpin := false
	return &xli.Command{Name: "use", Brief: "Select @edge/@stable or pin VERSION; force restart components (active work is interrupted)", Args: arg.Args{&arg.String{Name: "VERSION", Optional: true, Default: &selected}}, Flags: flg.Flags{&flg.Switch{Name: "unpin", Brief: "Remove the version pin without restarting", Default: &unpin}}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		rootCmd := c
		for rootCmd.HasParent() {
			rootCmd = rootCmd.Parent()
		}
		root, e := filepath.Abs(flg.MustGet[string](rootCmd, "state"))
		if e != nil {
			return e
		}
		version := arg.MustGet[string](c, "VERSION")
		clear := flg.MustGet[bool](c, "unpin")
		if clear && version != "" {
			return fmt.Errorf("--unpin does not accept VERSION")
		}
		if version == "" && !clear {
			p, e := useStatus(root)
			if e != nil {
				return e
			}
			return json.NewEncoder(c.Writer).Encode(p)
		}
		if !clear {
			if e = versionpin.ValidateSelection(version); e != nil {
				return e
			}
		}
		if e = os.MkdirAll(root, 0700); e != nil {
			return e
		}
		lock, e := core.Lock(filepath.Join(root, "self-update.lock"))
		if e != nil {
			return e
		}
		defer lock.Close()
		installation, e := core.Lock(filepath.Join(root, "install.lock"))
		if e != nil {
			return e
		}
		defer installation.Close()
		if clear {
			if p, e := loadUsePlan(root); e == nil {
				return fmt.Errorf("unfinished switch; retry cxz use %s", p.Requested)
			} else if !os.IsNotExist(e) {
				return e
			}
			if e = clearUseBackend(ctx, root); e != nil {
				return e
			}
			if e = versionpin.Clear(root); e != nil {
				return e
			}
			_, e = fmt.Fprintln(c.Writer, "Version pin removed. Installed processes are unchanged.")
			return e
		}
		return runUse(ctx, root, version, c.ErrWriter, c.Writer)
	})}
}
func runUse(ctx context.Context, root, requested string, progress, out io.Writer) error {
	plan, e := resolveUsePlan(ctx, root, requested)
	if e != nil {
		return e
	}
	version := plan.Pin.Version
	work, e := os.MkdirTemp("", "cxz-use-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(work)
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	replacement, e := selfupdate.PrepareWithDocker(ctx, exe, work, progress)
	if e != nil {
		return e
	}
	defer replacement.Close()
	var a selfupdate.Artifact
	if plan.Release != nil {
		a, e = selfupdate.DownloadChannel(ctx, work, runtime.GOOS, runtime.GOARCH, *plan.Release, progress)
	} else {
		a, e = selfupdate.DownloadRelease(ctx, work, version, runtime.GOOS, runtime.GOARCH, progress)
	}
	if e != nil {
		return e
	}
	if e = replacement.Stage(a.Path); e != nil {
		return e
	}
	a.Path = replacement.Candidate
	if e = verifyUpdatedExecutable(ctx, a, work); e != nil {
		return e
	}
	if plan.Release != nil {
		probe, cancel := context.WithTimeout(ctx, 10*time.Second)
		b, err := exec.CommandContext(probe, a.Path, "_use-capabilities").Output()
		cancel()
		var caps struct {
			Channels int `json:"channels"`
		}
		if err != nil || json.Unmarshal(b, &caps) != nil || caps.Channels != 1 {
			return fmt.Errorf("target release does not support channel selection")
		}
	}
	pin := plan.Pin
	pin.Revision = a.Revision
	if pin.Generation == "" {
		pin.Generation = core.ID()
		pin.At = time.Now()
	}
	plan.Pin = pin
	if e = core.WriteJSON(usePlanPath(root), plan); e != nil {
		return e
	}
	backend, e := prepareUseBackend(ctx, root, pin, progress)
	if e != nil {
		return e
	}
	if backend != nil {
		pin = backend.pin
	}
	if e = versionpin.Save(root, pin); e != nil {
		return e
	}
	if backend != nil {
		if e = backend.apply(ctx); e != nil {
			return fmt.Errorf("version switch incomplete; retry cxz use %s: %w", requested, e)
		}
	}
	// Every target process is serving before notifying local frontends.
	if runtime.GOOS == "windows" {
		replacement.Previous = replacement.Target + ".previous-" + pin.Generation + ".exe"
	}
	if _, e = replacement.Apply(); e != nil {
		return fmt.Errorf("server switch completed but client replacement failed; retry cxz use %s: %w", requested, e)
	}
	pin.Ready = true
	if e = versionpin.Save(root, pin); e != nil {
		return e
	}
	if backend != nil {
		if e = backend.finish(ctx); e != nil {
			return e
		}
	}
	if e = os.Remove(usePlanPath(root)); e != nil {
		return e
	}
	status, e := useStatus(root)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(status)
}
