package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/term"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/cxz/internal/workspace"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"github.com/lesomnus/xli/tab"
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
	path := stringArg("WORKSPACE", true)
	path.Default = ptr(".")
	path.Handler = arg.OnTab[string](func(_ context.Context, t tab.Tab) { t.Dirs() })
	c := &xli.Command{Name: name, Brief: map[string]string{"up": "Prepare project and attach existing session", "new": "Create project session and attach TUI", "down": "Remove owned containers, retaining volumes", "recreate": "Replace container; writable layer lost, editors disconnect"}[name], Args: arg.Args{path}, Handler: withClient(projectCommand)}
	if name != "down" {
		config := stringFlag("config", "Devcontainer configuration", "")
		config.Handler = flg.OnTab[string](func(_ context.Context, t tab.Tab) error { t.Files(""); return nil })
		c.Flags = flg.Flags{agentFlag(""), config, switchFlag("no-attach", "Return JSON without opening TUI"), switchFlag("trust-config", "Trust elevated settings and host initialization")}
	}
	if name == "recreate" {
		c.Flags = append(c.Flags, switchFlag("yes", "Confirm writable-layer loss and editor disconnection"))
	}
	return c
}

func projectCommand(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	command := c.Name
	path := arg.MustGet[string](c, "WORKSPACE")
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
		return json.NewEncoder(c.Writer).Encode(r)
	}
	agent := flg.MustGet[string](c, "agent")
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
	if command == "new" && agent == "" && terminal(c) {
		fmt.Fprint(c.ErrWriter, "Agent [claude/codex] (claude): ")
		v, e := bufio.NewReader(c.ReadCloser).ReadString('\n')
		if e != nil {
			return e
		}
		agent = strings.TrimSpace(v)
	}
	if command == "recreate" && !yes {
		projects, e := client.Projects(ctx, &api.Empty{})
		if e != nil {
			return e
		}
		fmt.Fprintln(c.ErrWriter, "Recreate removes the writable layer and disconnects attached editors. Workspace and named volumes are kept.")
		for _, p := range projects.Projects {
			if p.Workspace == path || p.Name == path || p.Id == path {
				fmt.Fprintf(c.ErrWriter, "Target: %s %s (%s)\n", p.ContainerId, p.Workspace, p.State)
			}
		}
		if !terminal(c) {
			return fmt.Errorf("review targets with cxz projects, then pass --yes")
		}
		fmt.Fprint(c.ErrWriter, "Type recreate to continue: ")
		v, e := bufio.NewReader(c.ReadCloser).ReadString('\n')
		if e != nil || strings.TrimSpace(v) != "recreate" {
			return fmt.Errorf("canceled")
		}
		yes = true
	}
	fmt.Fprintln(c.ErrWriter, "cxz: preparing workspace; initial image/agent downloads may take a few minutes")
	call, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	s, e := client.Open(call, &api.ProjectRequest{Workspace: path, Agent: agent, Config: config, NewSession: command == "new", Recreate: command == "recreate", Confirmed: yes, TrustConfig: trust, ClientId: core.ID()})
	if e != nil {
		return e
	}
	if detach || !terminal(c) {
		return json.NewEncoder(c.Writer).Encode(s)
	}
	return tui.RunSelected(ctx, client, s.Id)
}
func attach(ctx context.Context, client api.SessionsClient, arg string) error {
	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	list, e := client.List(c, &api.Empty{})
	if e != nil {
		return e
	}
	projectID := ""
	if arg != "" {
		if projects, e := client.Projects(c, &api.Empty{}); e == nil {
			for _, p := range projects.Projects {
				if p.Name == arg || p.Id == arg || p.Workspace == arg {
					if projectID != "" && projectID != p.Id {
						return fmt.Errorf("ambiguous project name; use its id")
					}
					projectID = p.Id
				}
			}
		}
	}
	var matches []*api.Session
	for _, s := range list.Sessions {
		if arg == "" || s.Id == arg || strings.HasPrefix(s.Id, arg) || s.ProjectId == arg || s.Title == arg || filepath.Base(s.Workspace) == arg || projectID != "" && s.ProjectId == projectID {
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
		return fmt.Errorf("no matching session; use cxz ls")
	}
	if len(matches) > 1 && arg != "" {
		return fmt.Errorf("ambiguous session; use a session id from cxz ls")
	}
	if len(matches) > 1 && arg == "" && os.Getenv("CXZ_PROJECT_ID") == "" {
		return tui.Run(ctx, client)
	}
	return tui.RunSelected(ctx, client, matches[0].Id)
}

func projectExec(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	op := c.Name
	name := arg.MustGet[string](c, "PROJECT")
	kind := "claude"
	var command []string
	if op == "login" {
		kind = flg.MustGet[string](c, "agent")
	} else {
		command, _ = arg.Get[[]string](c, "COMMAND")
	}
	if op == "exec" && len(command) == 0 {
		return fmt.Errorf("exec requires PROJECT -- COMMAND...")
	}
	projects, e := client.Projects(ctx, &api.Empty{})
	if e != nil {
		return e
	}
	var match *api.Project
	for _, p := range projects.Projects {
		if p.Id == name || p.Name == name || p.Workspace == name {
			if match != nil {
				return fmt.Errorf("ambiguous project name")
			}
			match = p
		}
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
	if op == "login" {
		flags = append(flags, "/cxz/tools/cxz", "--state", "/cxz/state/data", "_login", kind)
	} else {
		if len(command) == 0 {
			command = []string{"sh"}
		}
		flags = append(flags, command...)
	}
	process := exec.CommandContext(ctx, "docker", flags...)
	process.Stdin = c.ReadCloser
	process.Stdout = c.Writer
	process.Stderr = c.ErrWriter
	return process.Run()
}
