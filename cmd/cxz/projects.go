package main

import (
	"context"
	"errors"
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
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/cxz/resource"
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

func selectProjectAccount(ctx context.Context, resources *resourceclient.Client, c *xli.Command) (string, error) {
	if !terminal(c) {
		return "", fmt.Errorf("%s requires --account to create a session; list profiles with cxz account ls", c.Name)
	}
	var choices []*resource.Account
	after := ""
	for {
		page, err := resources.Accounts.List(ctx, resource.AccountListRequest_builder{Size: 200, After: after}.Build())
		if err != nil {
			return "", err
		}
		choices = append(choices, page.GetItems()...)
		after = page.GetNext()
		if after == "" {
			break
		}
	}
	if len(choices) == 0 {
		return "", fmt.Errorf("no accounts; run cxz account add codex NAME, then cxz account login NAME")
	}
	return tui.SelectAccount(ctx, choices, c.ReadCloser, c.ErrWriter)
}

func newProjectCommand(name string) *xli.Command {
	path := projectArg("WORKSPACE", true)
	path.Default = ptr(".")
	c := &xli.Command{Name: name, Brief: map[string]string{"up": "Prepare project and attach existing session", "new": "Create project session and attach TUI", "down": "Remove owned containers, retaining volumes", "recreate": "Replace container; writable layer lost, editors disconnect"}[name], Args: arg.Args{path}, Handler: withClient(projectCommand)}
	if name != "down" {
		config := stringFlag("config", "Devcontainer configuration", "")
		config.Handler = flg.OnTab[string](func(_ context.Context, t tab.Tab) error { t.Files(""); return nil })
		c.Flags = flg.Flags{agentFlag(""), stringFlag("model", "Model ID/alias for a new session (persisted on resume)", ""), config, switchFlag("no-attach", "Print session data without opening TUI (see --format)"), switchFlag("trust-config", "Trust elevated settings and host initialization")}
		c.Flags = append(c.Flags, stringFlag("name", "Project display name", ""), stringFlag("alias", "Unique short project handle (generated when omitted)", ""))
		c.Flags = append(c.Flags, stringFlag("account", "Registered profile; required for new sessions in scripts, selected interactively or inherited on resume", ""))
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
			if err = installer.SyncGitHub(ctx, stateFrom(ctx), c.ErrWriter, p); err != nil {
				return err
			}
			if flg.MustGet[bool](c, "no-attach") || !terminal(c) {
				return writeOutput(c, p)
			}
			return projectTUI(ctx, resources, c, p, "", flg.MustGet[bool](c, "trust-config"))
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
		var err error
		account, err = selectProjectAccount(ctx, client.(*resourceclient.Client), c)
		if err != nil {
			return err
		}
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
	detach := flg.MustGet[bool](c, "no-attach")
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
				if !terminal(c) {
					return fmt.Errorf("multiple configurations; pass --config")
				}
				for i, configPath := range configs {
					fmt.Fprintf(c.ErrWriter, "%d) %s\n", i+1, configPath)
				}
				fmt.Fprint(c.ErrWriter, "Configuration: ")
				var n int
				if _, e = fmt.Fscanln(c.ReadCloser, &n); e != nil || n < 1 || n > len(configs) {
					return fmt.Errorf("invalid configuration selection")
				}
				config = configs[n-1]
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
		if !errors.Is(err, resourceclient.ErrAccountRequired) {
			return err
		}
		request.Account, err = selectProjectAccount(ctx, resources, c)
		if err != nil {
			return err
		}
		if err = resources.CheckOpen(ctx, request); err != nil {
			return err
		}
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
		if !terminal(c) {
			return fmt.Errorf("review targets with cxz project ls, then pass --yes")
		}
		if err := tui.ConfirmRecreate(ctx, c.ReadCloser, c.ErrWriter, path); err != nil {
			return err
		}
		yes = true
	}
	request.Confirmed = yes
	fmt.Fprintln(c.ErrWriter, "cxz: preparing workspace; initial image/agent downloads may take a few minutes")
	call, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if projectEntry {
		if err := prepareWithProgress(call, c.ErrWriter, func(ctx context.Context) (*api.Project, error) {
			return resources.ResolveProject(ctx, path)
		}, func() error { _, err := resources.Open(call, request); return err }, time.Second); err != nil {
			return err
		}
		p, err := resources.ResolveProject(call, path)
		if err != nil {
			return err
		}
		if detach || !terminal(c) {
			return writeOutput(c, p)
		}
		return projectTUI(ctx, resources, c, p, "", trust)
	}
	s, e := openWithProjectLogin(call, request, terminal(c), func(ctx context.Context, r *api.ProjectRequest) (*api.Session, error) {
		return client.Open(ctx, r)
	}, func(ctx context.Context, alias, key string) error {
		a, err := resources.Account(ctx, alias)
		if err != nil {
			return err
		}
		fmt.Fprintln(c.ErrWriter, "cxz: workspace ready; starting project account login, then connecting the session")
		return projectAccountWorkflow(ctx, resources, c, a, "login", request.Workspace, false, key)
	}, c.ErrWriter)
	if e != nil {
		return e
	}
	if detach || !terminal(c) {
		return writeOutput(c, s)
	}
	p, err := resources.ResolveProject(ctx, s.ProjectId)
	if err != nil {
		return err
	}
	return projectTUI(ctx, resources, c, p, s.Id, trust)
}
func attach(ctx context.Context, client api.SessionsClient, arg string) error {
	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	list, e := client.List(c, &api.Empty{})
	if e != nil {
		return e
	}
	projectID := ""
	for _, s := range list.Sessions {
		if arg != "" && s.Alias == arg {
			return tui.RunSelected(ctx, client, s.Id)
		}
	}
	if arg != "" {
		if projects, e := client.Projects(c, &api.Empty{}); e == nil {
			if p, err := projectref.Resolve(projects.Projects, arg); err == nil {
				projectID = p.Id
			} else if status.Code(err) != codes.NotFound {
				return err
			}
		}
	}
	var matches []*api.Session
	for _, s := range list.Sessions {
		if projectID != "" && s.ProjectId == projectID || projectID == "" && (arg == "" || s.Id == arg || strings.HasPrefix(s.Id, arg) || s.ProjectId == arg || s.Title == arg || filepath.Base(s.Workspace) == arg) {
			matches = append(matches, s)
		}
	}
	if projectID != "" && len(matches) > 1 {
		for _, s := range matches {
			if s.State == "idle" || s.State == "working" || s.State == "waiting_input" {
				matches = []*api.Session{s}
				break
			}
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("no matching session; use cxz session ls")
	}
	if len(matches) > 1 && arg != "" {
		return fmt.Errorf("ambiguous session; use a session id from cxz session ls")
	}
	if len(matches) > 1 && arg == "" && os.Getenv("CXZ_PROJECT_ID") == "" {
		return tui.Run(ctx, client)
	}
	return tui.RunSelected(ctx, client, matches[0].Id)
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
	return parent
}
