//go:build windows

package main

import (
	"context"
	"fmt"
	"runtime"

	"github.com/lesomnus/cxz/internal/cxzupdate"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/cxz/internal/versionpin"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

func newRoot(state string) *xli.Command {
	root := &xli.Command{Name: "cxz", Brief: "Remote frontend for Linux cxz daemons", Flags: append(remoteFlags(), &flg.String{Name: "state", Brief: "Local client settings and recordings directory", Default: &state}), Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		return runRemote(versionpin.WithClient(cxzupdate.WithClient(ctx, flg.MustGet[string](c, "state")), flg.MustGet[string](c, "state")), flg.MustGet[string](c, "state"), flg.MustGet[string](c, "endpoint"), flg.MustGet[string](c, "token-file"), flg.MustGet[string](c, "session"))
	})}
	plain := false
	root.Commands = xli.Commands{connectionCommand(), aiCommand(), mcpCommand(), skillCommand(), editCommand(), devcontainerCommand(), selfUpdateCommand(), useCommand(), selfInstallCommand(), integrationCommand(),
		{Name: "terminal-info", Brief: "Print terminal environment and palette codes", Flags: flg.Flags{&flg.Switch{Name: "plain", Brief: "Print a text report", Default: &plain}}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			return tui.RunTerminalInfo(ctx, c.ReadCloser, c.Writer, true)
		})},
		{Name: "version", Brief: "Print frontend version", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error {
			_, err := fmt.Fprintf(c.Writer, "cxz %s %s/%s (remote frontend)\n", version, runtime.GOOS, runtime.GOARCH)
			return err
		})},
		xli.NewCmdCompletion(),
	}
	categorizeCommands(root)
	return root
}
