//go:build !windows

package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"os"
	"os/exec"
)

func aiLogin(ctx context.Context, c *xli.Command, r *api.AuxLoginInfoReply) error {
	install, e := transport.Load(stateFrom(ctx))
	if e != nil || install.Owner != r.Owner {
		return fmt.Errorf("run cxz ai login %s on the connected Manager host", r.Account)
	}
	args := []string{"exec", "-i", install.Container, "/cxz/tools/cxz", "--state", "/var/lib/cxz", "_auxiliary-manager-login", r.Account, r.Agent, r.Backend}
	if r.Backend == accounts.BrokeredAccessToken {
		args = []string{"exec", "-i", install.Container, "/cxz/tools/cxz", "--state", "/var/lib/cxz", "_central-account-login", r.Account}
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = c.ReadCloser
	cmd.Stdout = c.Writer
	cmd.Stderr = c.ErrWriter
	return cmd.Run()
}
func aiInternalCommands() []*xli.Command {
	makeCmd := func(name string, args arg.Args, fn commandFunc) *xli.Command {
		return &xli.Command{Name: name, Category: "Internal runtime", Args: args, Handler: onRun(fn)}
	}
	args := arg.Args{stringArg("ACCOUNT", false), stringArg("AGENT", false), stringArg("BACKEND", false)}
	profile := func(c *xli.Command) auxiliary.Profile {
		return auxiliary.Profile{Account: arg.MustGet[string](c, "ACCOUNT"), Agent: arg.MustGet[string](c, "AGENT"), Backend: arg.MustGet[string](c, "BACKEND")}
	}
	return []*xli.Command{
		makeCmd("_auxiliary-job", nil, func(ctx context.Context, c *xli.Command) error {
			return auxiliary.Serve(ctx, stateFrom(ctx), c.ReadCloser, c.Writer)
		}),
		makeCmd("_auxiliary-login", append(append(arg.Args{}, args...), stringArg("BINARY", false)), func(ctx context.Context, c *xli.Command) error {
			return auxiliary.Login(ctx, stateFrom(ctx), arg.MustGet[string](c, "BINARY"), profile(c), c.ReadCloser, c.Writer, c.ErrWriter)
		}),
		makeCmd("_auxiliary-manager-login", args, func(ctx context.Context, c *xli.Command) error {
			m := &workspace.Manager{Root: stateFrom(ctx), Owner: os.Getenv("CXZ_OWNER"), ToolsVolume: os.Getenv("CXZ_TOOLS_VOLUME"), Image: os.Getenv("CXZ_MANAGER_IMAGE")}
			return m.AuxiliaryLogin(ctx, profile(c), c.ReadCloser, c.Writer, c.ErrWriter)
		}),
	}
}
