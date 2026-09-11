package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/internal/distribution"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
	"runtime"
	"runtime/debug"
	"strings"
)

// Set by scripts/release.sh. A source build reports its VCS revision separately.
var version = "dev"

func releaseCommands() xli.Commands {
	return xli.Commands{
		{Name: "version", Brief: "Print client build and pinned agent versions", Handler: onRun(func(_ context.Context, c *xli.Command) error {
			revision := ""
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
			return json.NewEncoder(c.Writer).Encode(map[string]any{"version": version, "revision": revision, "dirty": dirty, "platform": runtime.GOOS + "/" + runtime.GOARCH, "claude": distribution.ClaudeVersion, "codex": distribution.CodexVersion})
		})},
		{Name: "update", Brief: "Replace manager with an explicit pinned image; keep projects/data", Flags: flg.Flags{stringFlag("image", "Required version tag or digest (not latest)", "")}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			image := flg.MustGet[string](c, "image")
			last := image[strings.LastIndex(image, "/")+1:]
			if image == "" || (!strings.Contains(last, ":") && !strings.Contains(last, "@")) || strings.HasSuffix(image, ":latest") {
				return fmt.Errorf("update requires --image with an explicit version tag or digest")
			}
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
