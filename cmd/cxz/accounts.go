package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/cxz/resource"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"google.golang.org/protobuf/encoding/protojson"
)

func accountCommands() *xli.Command {
	parent := &xli.Command{Name: "account", Brief: "Register isolated agent authentication profiles", Handler: onRun(func(_ context.Context, c *xli.Command) error { return c.PrintHelp(c.Writer) })}
	for _, op := range []string{"add", "list", "get", "login", "status"} {
		c := &xli.Command{Name: op, Handler: withClient(accountCommand)}
		if op != "list" {
			c.Args = arg.Args{stringArg("ACCOUNT", false)}
		}
		if op == "add" {
			c.Flags = flg.Flags{agentFlag(""), stringFlag("name", "Display name", "")}
		}
		if op == "login" || op == "status" {
			c.Flags = flg.Flags{stringFlag("project", "Project alias or workspace (defaults to current directory)", ".")}
		}
		parent.Commands = append(parent.Commands, c)
	}
	return parent
}
func accountCommand(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	resources := client.(*resourceclient.Client)
	if c.Name == "list" {
		after := ""
		var all []*resource.Account
		for {
			page, err := resources.Accounts.List(ctx, resource.AccountListRequest_builder{Size: 200, After: after}.Build())
			if err != nil {
				return err
			}
			all = append(all, page.GetItems()...)
			after = page.GetNext()
			if after == "" {
				fmt.Fprintln(c.Writer, protojson.Format(resource.AccountListResponse_builder{Items: all}.Build()))
				return nil
			}
		}
	}
	alias := arg.MustGet[string](c, "ACCOUNT")
	if c.Name == "add" {
		a, err := resources.Accounts.Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Name: flg.MustGet[string](c, "name"), Agent: flg.MustGet[string](c, "agent")}.Build())
		if err != nil {
			return err
		}
		fmt.Fprintln(c.Writer, protojson.Format(a))
		return nil
	}
	a, err := resources.Account(ctx, alias)
	if err != nil {
		return err
	}
	if c.Name == "get" {
		fmt.Fprintln(c.Writer, protojson.Format(a))
		return nil
	}
	// Account registration is global; OAuth grants belong to one project and
	// profile. Never fan out rotating refresh tokens (architecture §4.2).
	install, err := transport.Load(stateFrom(ctx))
	if err != nil {
		return fmt.Errorf("account login/status requires cxz install: %w", err)
	}
	target := flg.MustGet[string](c, "project")
	if st, e := os.Stat(target); e == nil && st.IsDir() {
		target, err = dockerx.EnginePath(target)
		if err != nil {
			return err
		}
	}
	if c.Name == "login" {
		if _, err = resources.Open(ctx, &api.ProjectRequest{Workspace: target, Agent: a.GetAgent(), PrepareOnly: true, ClientId: core.ID()}); err != nil {
			return err
		}
	}
	p, err := resources.ResolveProject(ctx, target)
	if err != nil {
		return err
	}
	if p.State != "running" {
		return fmt.Errorf("project is not running; use cxz up first")
	}
	if _, err = dockerx.Owned(ctx, p.ContainerId, install.Owner, p.Id); err != nil {
		return err
	}
	if c.Name == "login" {
		list, err := client.List(ctx, &api.Empty{})
		if err != nil {
			return err
		}
		for _, s := range list.Sessions {
			if s.ProjectId == p.Id && (s.State == "idle" || s.State == "working" || s.State == "waiting_input" || s.State == "starting") {
				return fmt.Errorf("stop session %s before logging in this account again", s.Id)
			}
		}
		fmt.Fprintf(c.ErrWriter, "Log in as %s (%s) for project %s. Other projects require independent login.\n", a.GetAlias(), a.GetAgent(), p.Alias)
	}
	args := []string{"exec", "-i"}
	if terminal(c) {
		args = append(args, "-t")
	}
	args = append(args, "--user", p.RemoteUser, p.ContainerId, "/cxz/tools/cxz", "--state", "/cxz/state/data", "_account-"+c.Name, a.GetAlias(), a.GetAgent())
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = c.ReadCloser
	cmd.Stdout = c.Writer
	cmd.Stderr = c.ErrWriter
	return cmd.Run()
}

func accountInternalCommands() []*xli.Command {
	var out []*xli.Command
	for _, op := range []string{"login", "import", "status"} {
		c := &xli.Command{Name: "_account-" + op, Category: "Internal runtime", Args: arg.Args{stringArg("ACCOUNT", false), stringArg("AGENT", false)}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			alias, agent := arg.MustGet[string](c, "ACCOUNT"), arg.MustGet[string](c, "AGENT")
			if err := accounts.Validate(alias, agent); err != nil {
				return err
			}
			root := stateFrom(ctx)
			if c.Name == "_account-status" {
				_, err := accounts.Credential(root, alias, agent)
				if err != nil {
					return err
				}
				fmt.Fprintln(c.Writer, "Credential file present; vendor validity is checked when the agent connects.")
				return nil
			}
			if c.Name == "_account-import" {
				b, err := io.ReadAll(io.LimitReader(c.ReadCloser, 1024*1024+1))
				if err != nil {
					return err
				}
				if len(b) > 1024*1024 {
					return fmt.Errorf("credential transfer too large")
				}
				return accounts.Install(root, alias, agent, b)
			}
			r, err := workspace.LoadRuntime(root)
			if err != nil {
				return err
			}
			workspaceLock, err := core.Lock(filepath.Join(root, "run", fmt.Sprintf("workspace-%x.lock", sha256.Sum256([]byte(r.Workspace)))))
			if err != nil {
				return fmt.Errorf("stop the active project session before login: %w", err)
			}
			defer workspaceLock.Close()
			if err := accounts.Prepare(root, alias, agent); err != nil {
				return err
			}
			lock, err := core.Lock(filepath.Join(accounts.Dir(root, alias), "login.lock"))
			if err != nil {
				return err
			}
			defer lock.Close()
			// A canceled/failed login cannot replace the last working profile.
			staging, err := os.MkdirTemp(accounts.Dir(root, alias), "login-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(staging)
			if err = accounts.Prepare(staging, alias, agent); err != nil {
				return err
			}
			bin := r.Claude
			if agent == "codex" {
				bin = r.Codex
			}
			if bin == "" {
				return fmt.Errorf("agent not provisioned")
			}
			args := []string{"auth", "login"}
			if agent == "codex" {
				args = []string{"-c", `cli_auth_credentials_store="file"`, "login", "--device-auth"}
			}
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Dir = accounts.Dir(staging, alias)
			cmd.Env = accounts.Environment(os.Environ(), staging, alias, agent)
			cmd.Stdin = c.ReadCloser
			cmd.Stdout = c.Writer
			cmd.Stderr = c.ErrWriter
			if err = cmd.Run(); err != nil {
				return err
			}
			credential, err := accounts.Credential(staging, alias, agent)
			if err != nil {
				return err
			}
			return accounts.Install(root, alias, agent, credential)
		})}
		out = append(out, c)
	}
	return out
}
