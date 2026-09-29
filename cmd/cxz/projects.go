//go:build !windows

package main

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/term"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/projectref"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"github.com/lesomnus/xli/tab"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func terminal(c *xli.Command) bool {
	f, ok := c.ReadCloser.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

func newProjectCommand(name string) *xli.Command {
	path := projectArg("WORKSPACE", true)
	path.Default = ptr(".")
	c := &xli.Command{Name: name, Brief: map[string]string{"up": "Prepare project and print result", "new": "Create project session and print result", "down": "Remove owned containers; retain project, sessions, history and volumes", "recreate": "Replace container; writable layer lost, editors disconnect"}[name], Args: arg.Args{path}, Handler: withClient(projectCommand)}
	if name != "down" {
		config := stringFlag("config", "Devcontainer configuration", "")
		config.Handler = flg.OnTab[string](func(_ context.Context, t tab.Tab) error { t.Files(""); return nil })
		c.Flags = flg.Flags{agentFlag(""), stringFlag("model", "Model ID/alias for a new session (persisted on resume)", ""), config, switchFlag("trust-config", "Trust elevated settings and host initialization")}
		c.Flags = append(c.Flags, stringFlag("name", "Project display name", ""), stringFlag("alias", "Unique short project handle (generated when omitted)", ""))
		c.Flags = append(c.Flags, stringFlag("account", "Registered profile; required for new sessions", ""))
	}
	if name == "recreate" {
		c.Flags = append(c.Flags, switchFlag("yes", "Confirm writable-layer loss and editor disconnection"))
	}
	return c
}

func projectArg(name string, optional bool) *arg.String {
	return stringArg(name, optional)
}
func bindProjectCompletions(root *xli.Command) {
	handler := arg.OnTab[string](func(ctx context.Context, t tab.Tab) {
		t.Dirs()
		if root.Flags.Get("state") == nil {
			return
		}
		state := flg.MustGet[string](root, "state")
		if state == "" {
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
		defer cancel()
		conn, err := transport.Dial(state)
		if err != nil {
			return
		}
		defer conn.Close()
		projects, err := resourceclient.New(conn).Projects(ctx, &api.Empty{})
		if err != nil {
			return
		}
		for _, p := range projects.Projects {
			if p.State == "foreign" {
				continue
			}
			if p.Alias != "" {
				t.ValueD(p.Alias, p.Name+" · "+p.Workspace)
			}
			t.ValueD(p.Name, p.Workspace)
			t.ValueD(p.Id, p.Name)
		}
	})
	var walk func(*xli.Command)
	walk = func(c *xli.Command) {
		for _, a := range c.Args {
			if v, ok := a.(*arg.String); ok && (v.Name == "PROJECT" || v.Name == "WORKSPACE" || v.Name == "TARGET") {
				v.Handler = handler
			}
		}
		for _, child := range c.Commands {
			walk(child)
		}
	}
	walk(root)
}

func projectCommand(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	command := c.Name
	projectEntry := command == "up" && c.Parent().Name == "cxz"
	path := arg.MustGet[string](c, "WORKSPACE")
	if projectEntry {
		target := path
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			var e error
			target, e = dockerx.EnginePath(target)
			if e != nil {
				return e
			}
		}
		resources := client.(*resourceclient.Client)
		p, err := resources.ResolveProject(ctx, target)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		changed := false
		for _, flag := range []string{"config", "name", "alias", "agent"} {
			if _, set := flg.Get[string](c, flag); set {
				changed = true
			}
		}
		if err == nil && p.State == "running" && !changed {
			if _, err := syncDevcontainer(ctx, client, stateFrom(ctx), settings.From(ctx)); err != nil {
				return err
			}
			if err = installer.SyncGitHub(ctx, stateFrom(ctx), c.ErrWriter, p); err != nil {
				return err
			}
			return workspaceReady(c, p)
		}
	}
	if command == "down" {
		if st, e := os.Stat(path); e == nil && st.IsDir() {
			path, e = dockerx.EnginePath(path)
			if e != nil {
				return e
			}
		}
		r, e := client.Down(ctx, &api.ProjectRequest{Workspace: path, ClientId: core.ID()})
		if e != nil {
			return e
		}
		return writeOutput(c, r)
	}
	if err := installer.SyncGitHub(ctx, stateFrom(ctx), c.ErrWriter); err != nil {
		return err
	}
	agent := flg.MustGet[string](c, "agent")
	account, _ := flg.Get[string](c, "account")
	if command == "new" && account == "" {
		return fmt.Errorf("new requires --account; list profiles with cxz account ls")
	}

	if account != "" {
		a, err := client.(*resourceclient.Client).Account(ctx, account)
		if err != nil {
			return err
		}
		if agent != "" && agent != a.GetAgent() {
			return fmt.Errorf("agent does not match account")
		}
		agent = a.GetAgent()
	}
	model, _ := flg.Get[string](c, "model")
	cfg := settings.From(ctx)
	config := flg.MustGet[string](c, "config")
	trust := flg.MustGet[bool](c, "trust-config")
	yes := false
	if command == "recreate" {
		yes = flg.MustGet[bool](c, "yes")
	}
	local := path
	if st, e := os.Stat(path); e == nil && st.IsDir() {
		var e error
		path, e = dockerx.EnginePath(path)
		if e != nil {
			return e
		}
		if config == "" {
			configs := workspace.Discover(local)
			if len(configs) > 1 {
				return fmt.Errorf("multiple configurations; pass --config")
			}

		}
	}
	if config != "" {
		v := config
		if !filepath.IsAbs(v) {
			if _, e := os.Stat(v); e != nil {
				v = filepath.Join(local, v)
			}
		}
		var e error
		config, e = dockerx.EnginePath(v)
		if e != nil {
			return e
		}
	}
	if command == "new" && model == "" {
		model = cfg.Model(agent)
	}
	request := &api.ProjectRequest{Workspace: path, Name: flg.MustGet[string](c, "name"), Alias: flg.MustGet[string](c, "alias"), Agent: agent, Model: model, Config: config, NewSession: command == "new", Recreate: command == "recreate", Confirmed: yes, TrustConfig: trust, ClientId: core.ID(), Account: account}
	request.PrepareOnly = projectEntry
	resources := client.(*resourceclient.Client)
	if err := resources.CheckOpen(ctx, request); err != nil {
		return err
	}

	if _, err := syncDevcontainer(ctx, client, stateFrom(ctx), cfg); err != nil {
		return err
	}
	if command == "recreate" && !yes {
		projects, e := client.Projects(ctx, &api.Empty{})
		if e != nil {
			return e
		}
		fmt.Fprintln(c.ErrWriter, "Recreate removes the writable layer and disconnects attached editors. Workspace and named volumes are kept.")
		if p, err := projectref.Resolve(projects.Projects, path); err == nil {
			fmt.Fprintf(c.ErrWriter, "Target: %s %s (%s)\n", p.ContainerId, p.Workspace, p.State)
		} else if status.Code(err) != codes.NotFound {
			return err
		} else {
			for _, p := range projects.Projects {
				if p.State == "foreign" && p.Workspace == path {
					fmt.Fprintf(c.ErrWriter, "Target: %s %s (foreign)\n", p.ContainerId, p.Workspace)
				}
			}
		}
		return fmt.Errorf("review targets with cxz project ls, then pass --yes")
	}
	request.Confirmed = yes
	fmt.Fprintln(c.ErrWriter, "cxz: preparing workspace; initial image/agent downloads may take a few minutes")
	call, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if projectEntry {
		_, err := withConfigTrust(call, request, interactiveErrors(c), func(ctx context.Context, r *api.ProjectRequest) (*api.Session, error) {
			err := prepareWithProgress(ctx, c.ErrWriter, func(ctx context.Context) (*api.Project, error) { return resources.ResolveProject(ctx, path) }, func() error { _, err := resources.Open(ctx, r); return err }, time.Second)
			return nil, err
		}, trustPrompt(c))
		if err != nil {
			return err
		}

		p, err := resources.ResolveProject(call, path)
		if err != nil {
			return err
		}
		return workspaceReady(c, p)
	}
	s, e := openWithProjectLogin(call, request, terminal(c) && !exitOnError(c), func(ctx context.Context, r *api.ProjectRequest) (*api.Session, error) {
		return withConfigTrust(ctx, r, interactiveErrors(c), func(ctx context.Context, r *api.ProjectRequest) (*api.Session, error) {
			if !terminal(c) || exitOnError(c) {
				return client.Open(ctx, r)
			}
			var session *api.Session
			err := sessionProgress(ctx, c.ErrWriter, func() error { var err error; session, err = client.Open(ctx, r); return err })
			return session, err
		}, trustPrompt(c))
	}, func(ctx context.Context, alias, key string) error {
		a, err := resources.Account(ctx, alias)
		if err != nil {
			return err
		}
		fmt.Fprintln(c.ErrWriter, "cxz: workspace ready; starting project account login, then connecting the session")
		return projectAccountWorkflow(ctx, resources, c, a, "login", request.Workspace, false, key)
	})
	if e != nil {
		return e
	}
	return writeOutput(c, s)
}

func projectExec(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	op := c.Name
	name := arg.MustGet[string](c, "PROJECT")
	var command []string
	command, _ = arg.Get[[]string](c, "COMMAND")
	if op == "exec" && len(command) == 0 {
		return fmt.Errorf("exec requires PROJECT -- COMMAND...")
	}
	projects, e := client.Projects(ctx, &api.Empty{})
	if e != nil {
		return e
	}
	match, e := projectref.Resolve(projects.Projects, name)
	if e != nil {
		return e
	}
	if match == nil || match.State != "running" {
		return fmt.Errorf("no running owned project %s", name)
	}
	flags := []string{"exec", "-i"}
	if terminal(c) {
		flags = append(flags, "-t")
	}
	user := match.RemoteUser
	if user == "" {
		user = "root"
	}
	flags = append(flags, "--user", user, "--workdir", match.RemoteWorkspace, match.ContainerId)
	if len(command) == 0 {
		command = []string{"sh"}
	}
	flags = append(flags, command...)
	process := exec.CommandContext(ctx, "docker", flags...)
	process.Stdin = c.ReadCloser
	process.Stdout = c.Writer
	process.Stderr = c.ErrWriter
	return process.Run()
}

func projectMetadataCommands() *xli.Command {
	parent := &xli.Command{Name: "project", Brief: "Register projects and set their display names/aliases", Handler: onRun(func(_ context.Context, c *xli.Command) error { return c.PrintHelp(c.Writer) })}
	for _, op := range []string{"add", "set"} {
		c := &xli.Command{Name: op, Brief: map[string]string{"add": "Register a workspace without starting a container", "set": "Change project display name or alias"}[op], Args: arg.Args{stringArg("PROJECT", false)}, Flags: flg.Flags{&flg.String{Name: "name", Brief: "Display name"}, &flg.String{Name: "alias", Brief: "Unique short handle"}}, Handler: withClient(func(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
			target := arg.MustGet[string](c, "PROJECT")
			if st, err := os.Stat(target); err == nil && st.IsDir() {
				target, err = dockerx.EnginePath(target)
				if err != nil {
					return err
				}
			}
			name, hasName := flg.Get[string](c, "name")
			alias, hasAlias := flg.Get[string](c, "alias")
			if hasName && strings.TrimSpace(name) == "" || hasAlias && strings.TrimSpace(alias) == "" {
				return fmt.Errorf("name and alias cannot be empty")
			}
			resources := client.(*resourceclient.Client)
			var p *api.Project
			var err error
			if c.Name == "add" {
				p, err = resources.AddProject(ctx, target, name, alias, "")
			} else {
				if !hasName && !hasAlias {
					return fmt.Errorf("set requires --name or --alias")
				}
				var n, a *string
				if hasName {
					n = &name
				}
				if hasAlias {
					a = &alias
				}
				p, err = resources.SetProject(ctx, target, n, a)
			}
			if err != nil {
				return err
			}
			return writeOutput(c, p)
		})}
		parent.Commands = append(parent.Commands, c)
	}
	parent.Commands = append(parent.Commands, projectRemoveCommand())
	return parent
}

func projectRemoveCommand() *xli.Command {
	return &xli.Command{Name: "rm", Brief: "Permanently delete one project and its owned data; keep workspace sources", Args: arg.Args{projectArg("PROJECT", false)}, Flags: flg.Flags{switchFlag("yes", "Confirm permanent deletion of sessions, history, memory and owned volumes")}, Handler: withClient(func(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
		target := arg.MustGet[string](c, "PROJECT")
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			var err error
			target, err = dockerx.EnginePath(target)
			if err != nil {
				return err
			}
		}
		confirmed := flg.MustGet[bool](c, "yes")
		out, err := client.(*resourceclient.Client).RemoveProject(ctx, target, confirmed)
		if err != nil {
			return err
		}
		p := out.GetProject()
		if !confirmed {
			fmt.Fprintf(c.ErrWriter, "Target: %s %s (%s)\n", p.GetRuntimeId(), p.GetWorkspace(), p.GetName())
			return fmt.Errorf("project rm permanently deletes this project's sessions, history, memory and owned resources; workspace sources and shared accounts are kept; pass --yes to confirm")
		}
		if !out.GetRemoved() {
			return fmt.Errorf("manager did not confirm project removal")
		}
		fmt.Fprintf(c.Writer, "Removed project %s. Workspace sources preserved: %s\n", p.GetRuntimeId(), p.GetWorkspace())
		return nil
	})}
}
