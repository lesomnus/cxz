//go:build !windows

package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"runtime"
	"runtime/debug"
)

func releaseCommands() xli.Commands {
	return xli.Commands{
		{Name: "version", Brief: "Print client build and pinned agent versions", Handler: onRun(func(_ context.Context, c *xli.Command) error {
			revision := buildRevision
			dirty := false
			if info, ok := debug.ReadBuildInfo(); ok {
				for _, s := range info.Settings {
					if s.Key == "vcs.revision" {
						revision = s.Value
					}
					if s.Key == "vcs.modified" {
						dirty = s.Value == "true"
					}
				}
			}
			return writeOutput(c, map[string]any{"version": version, "revision": revision, "dirty": dirty, "platform": runtime.GOOS + "/" + runtime.GOARCH, "claude": distribution.ClaudeVersion, "codex": distribution.CodexVersion})
		})},
		{Name: "update", Brief: "Replace manager with IMAGE (explicit tag/digest, not latest); keep projects/data", Args: arg.Args{stringArg("IMAGE", false)}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			image := arg.MustGet[string](c, "IMAGE")
			v, err := transport.Load(stateFrom(ctx))
			if err != nil {
				return err
			}
			return installer.Install(ctx, stateFrom(ctx), v.WorkspaceRoot, image, true, c.ErrWriter)
		})},
		{Name: "rollback", Brief: "Replace manager with recorded previous image; retain data", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			v, err := transport.Load(stateFrom(ctx))
			if err != nil {
				return err
			}
			if v.PreviousImage == "" {
				return fmt.Errorf("no previous manager image recorded")
			}
			return installer.Install(ctx, stateFrom(ctx), v.WorkspaceRoot, v.PreviousImage, true, c.ErrWriter)
		})},
	}
}
