//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

func newRoot(state string) *xli.Command {
	root := &xli.Command{Name: "cxz", Brief: "Remote frontend for Linux cxz daemons", Flags: flg.Flags{&flg.String{Name: "state", Brief: "Local client settings and recordings directory", Default: &state}}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		return runRemote(ctx, flg.MustGet[string](c, "state"), os.Getenv("CXZ_ENDPOINT"), os.Getenv("CXZ_TOKEN_FILE"), "")
	})}
	plain := false
	root.Commands = xli.Commands{connectCommand(), editCommand(),
		{Name: "terminal-info", Brief: "Inspect terminal environment and interactive palette", Flags: flg.Flags{&flg.Switch{Name: "plain", Brief: "Print a text report", Default: &plain}}, Handler: xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			return tui.RunTerminalInfo(ctx, c.ReadCloser, c.Writer, flg.MustGet[bool](c, "plain"))
		})},
		{Name: "version", Brief: "Print frontend version", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error {
			_, err := fmt.Fprintf(c.Writer, "cxz %s %s/%s (remote frontend)\n", version, runtime.GOOS, runtime.GOARCH)
			return err
		})},
		xli.NewCmdCompletion(),
	}
	return root
}
