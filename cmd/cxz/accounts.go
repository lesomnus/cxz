package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/accounts"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/distribution"
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
	for _, op := range []string{"add", "list", "get", "login", "status", "bindings"} {
		c := &xli.Command{Name: op, Handler: withClient(accountCommand)}
		if op != "list" {
			c.Args = arg.Args{stringArg("ACCOUNT", false)}
		}
		if op == "add" {
			c.Args = arg.Args{agentArg(), stringArg("ACCOUNT", false)}
			c.Flags = flg.Flags{stringFlag("name", "Display name (Account alias when omitted)", ""), stringFlag("auth-backend", "Strategy (codex: brokered-access-token; claude: project-local-oauth)", "")}
		}
		if op == "login" || op == "status" {
			c.Flags = flg.Flags{stringFlag("project", "For project-local OAuth only; defaults to current directory. Central login needs no project", ".")}
		}
		parent.Commands = append(parent.Commands, c)
	}
	parent.Commands = append(parent.Commands, &xli.Command{Name: "backends", Brief: "List supported agent/auth backend mappings", Handler: onRun(func(_ context.Context, c *xli.Command) error {
		return json.NewEncoder(c.Writer).Encode(accounts.Catalog())
	})})
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
		a, err := resources.Accounts.Add(ctx, resource.AccountAddRequest_builder{Alias: alias, Name: flg.MustGet[string](c, "name"), Agent: arg.MustGet[string](c, "AGENT"), AuthBackend: flg.MustGet[string](c, "auth-backend")}.Build())
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
	if c.Name == "bindings" {
		after := ""
		var all []*resource.AuthBinding
		for {
			page, err := resources.Bindings.List(ctx, resource.AuthBindingListRequest_builder{Filters: []*resource.AuthBindingFilter{resource.AuthBindingFilter_builder{Account: resource.AccountRef_builder{Id: a.GetId()}.Build()}.Build()}, Size: 200, After: after}.Build())
			if err != nil {
				return err
			}
			all = append(all, page.GetItems()...)
			after = page.GetNext()
			if after == "" {
				fmt.Fprintln(c.Writer, protojson.Format(resource.AuthBindingListResponse_builder{Items: all}.Build()))
				return nil
			}
		}
	}
	backend, err := accounts.Resolve(a.GetAgent(), a.GetAuthBackend())
	if err != nil {
		return err
	}
	if backend.Info().Workflow == "account-login" {
		if _, set := flg.Get[string](c, "project"); set {
			return fmt.Errorf("central account login/status does not accept --project; omit it")
		}
		install, err := transport.Load(stateFrom(ctx))
		if err != nil {
			return err
		}
		args := []string{"exec", "-i"}
		if terminal(c) {
			args = append(args, "-t")
		}
		args = append(args, install.Container, "/cxz/tools/cxz", "--state", "/var/lib/cxz", "_central-account-"+c.Name, alias)
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Stdin = c.ReadCloser
		cmd.Stdout = c.Writer
		cmd.Stderr = c.ErrWriter
		return cmd.Run()
	}
	if backend.Info().Workflow != "project-login" || backend.Info().Scope != "project" {
		return fmt.Errorf("unsupported auth workflow: %s", backend.Info().Workflow)
	}
	return projectAccountWorkflow(ctx, resources, c, a, c.Name, flg.MustGet[string](c, "project"), true)
}

func projectAccountWorkflow(ctx context.Context, resources *resourceclient.Client, c *xli.Command, a *resource.Account, op, target string, prepare bool) error {
	backend, err := accounts.Resolve(a.GetAgent(), a.GetAuthBackend())
	if err != nil {
		return err
	}
	if backend.Info().Workflow != "project-login" || backend.Info().Scope != "project" {
		return fmt.Errorf("account does not use project login")
	}
	// Account registration is global; OAuth grants belong to one project and
	// profile. Never fan out rotating refresh tokens (architecture §4.2).
	install, err := transport.Load(stateFrom(ctx))
	if err != nil {
		return fmt.Errorf("account login/status requires cxz install: %w", err)
	}
	if st, e := os.Stat(target); e == nil && st.IsDir() {
		target, err = dockerx.EnginePath(target)
		if err != nil {
			return err
		}
	}
	if op == "login" && prepare {
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
	if op == "login" {
		list, err := resources.List(ctx, &api.Empty{})
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
	var binding *resource.AuthBinding
	if op == "login" {
		binding, err = resources.Bind(ctx, p.Id, a.GetAlias())
	} else {
		spec, e := backend.Binding(p.Id, a.GetAlias())
		if e != nil {
			return e
		}
		id := spec.ID
		binding, err = resources.Bindings.Get(ctx, resource.AuthBindingGetRequest_builder{Ref: resource.AuthBindingRef_builder{BindingId: &id}.Build(), Select: resource.AuthBindingSelect_builder{All: ptr(true)}.Build()}.Build())
	}
	if err != nil {
		return err
	}
	if op == "login" {
		fmt.Fprintf(c.ErrWriter, "Authentication backend: %s; binding: %s\n", binding.GetAuthBackend(), binding.GetBindingId())
	}
	args := []string{"exec", "-i"}
	if terminal(c) {
		args = append(args, "-t")
	}
	args = append(args, "--user", p.RemoteUser, p.ContainerId, "/cxz/tools/cxz", "--state", "/cxz/state/data", "_account-"+op, a.GetAlias(), a.GetAgent(), a.GetAuthBackend())
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = c.ReadCloser
	cmd.Stdout = c.Writer
	if !prepare {
		cmd.Stdout = c.ErrWriter
	} // Inline login must not contaminate session JSON.
	cmd.Stderr = c.ErrWriter
	return cmd.Run()
}

func accountInternalCommands() []*xli.Command {
	var out []*xli.Command
	for _, op := range []string{"login", "status"} {
		out = append(out, &xli.Command{Name: "_central-account-" + op, Category: "Internal runtime", Args: arg.Args{stringArg("ACCOUNT", false)}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			if os.Getenv("CXZ_OWNER") == "" {
				return fmt.Errorf("central authentication requires manager")
			}
			alias := arg.MustGet[string](c, "ACCOUNT")
			if err := accounts.Validate(alias, "codex"); err != nil {
				return err
			}
			if c.Name == "_central-account-status" {
				_, err := accounts.CentralToken(stateFrom(ctx), alias)
				if err == nil {
					fmt.Fprintln(c.Writer, "Central Codex login present; tokens are supplied only to connected projects.")
				}
				return err
			}
			bin, err := distribution.Ensure(ctx, "/cxz/tools", "codex", "", true)
			if err != nil {
				return err
			}
			fmt.Fprintln(c.ErrWriter, "Log in to the selected Codex account. This login is shared by its authorized projects.")
			return accounts.CentralLogin(ctx, accounts.LoginRequest{Root: stateFrom(ctx), Account: alias, Binary: bin, Env: os.Environ(), Input: c.ReadCloser, Output: c.Writer, Error: c.ErrWriter})
		})})
	}
	out = append(out, &xli.Command{Name: "_account-bind", Category: "Internal runtime", Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		var g accounts.Grant
		if err := json.NewDecoder(io.LimitReader(c.ReadCloser, 16384)).Decode(&g); err != nil {
			return fmt.Errorf("invalid account connection")
		}
		r, err := workspace.LoadRuntime(stateFrom(ctx))
		if err != nil {
			return err
		}
		if r.ProjectID != g.Project {
			return fmt.Errorf("account connection project mismatch")
		}
		return accounts.InstallGrant(stateFrom(ctx), g)
	})})
	for _, op := range []string{"login", "import", "status"} {
		c := &xli.Command{Name: "_account-" + op, Category: "Internal runtime", Args: arg.Args{stringArg("ACCOUNT", false), stringArg("AGENT", false), &arg.String{Name: "BACKEND", Optional: true, Default: ptr(accounts.ProjectLocalOAuth)}}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
			alias, agent := arg.MustGet[string](c, "ACCOUNT"), arg.MustGet[string](c, "AGENT")
			if err := accounts.Validate(alias, agent); err != nil {
				return err
			}
			backend, err := accounts.Resolve(agent, arg.MustGet[string](c, "BACKEND"))
			if err != nil {
				return err
			}
			root := stateFrom(ctx)
			if c.Name == "_account-status" {
				err := backend.Check(root, alias)
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
			bin := r.Claude
			if accounts.AgentKind(agent) == accounts.Codex {
				bin = r.Codex
			}
			return backend.Login(ctx, accounts.LoginRequest{Root: root, Account: alias, Workspace: r.Workspace, Binary: bin, Env: os.Environ(), Input: c.ReadCloser, Output: c.Writer, Error: c.ErrWriter})
		})}
		out = append(out, c)
	}
	return out
}
