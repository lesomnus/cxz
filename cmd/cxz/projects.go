package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/cxz/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func terminal() bool { st, e := os.Stdin.Stat(); return e == nil && st.Mode()&os.ModeCharDevice != 0 }

// Keep conventional `cxz up . --agent codex` ordering with the standard flag package.
func optionsFirst(args []string, bools map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		v := args[i]
		if strings.HasPrefix(v, "-") {
			flags = append(flags, v)
			if !strings.Contains(v, "=") && !bools[strings.TrimLeft(v, "-")] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, v)
		}
	}
	return append(flags, pos...)
}
func projectCommand(ctx context.Context, client api.SessionsClient, command string, args []string) error {
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	agent := f.String("agent", "", "claude or codex; remembered on existing sessions")
	config := f.String("config", "", "devcontainer configuration")
	detach := f.Bool("no-attach", false, "prepare session without opening the TUI")
	yes := f.Bool("yes", false, "confirm replacement of matching containers")
	trust := f.Bool("trust-config", false, "explicitly trust privileged settings/host lifecycle commands")
	if e := f.Parse(optionsFirst(args, map[string]bool{"no-attach": true, "yes": true, "trust-config": true})); e != nil {
		return e
	}
	path := "."
	if f.NArg() > 0 {
		path = f.Arg(0)
	}
	if f.NArg() > 1 {
		return fmt.Errorf("one workspace argument expected")
	}
	if command == "down" {
		r, e := client.Down(ctx, &api.ProjectRequest{Workspace: path, ClientId: core.ID()})
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(r)
	}
	local := path
	if st, e := os.Stat(path); e == nil && st.IsDir() {
		var e error
		path, e = dockerx.EnginePath(path)
		if e != nil {
			return e
		}
		if *config == "" {
			configs := workspace.Discover(local)
			if len(configs) > 1 {
				if !terminal() {
					return fmt.Errorf("multiple configurations; pass --config")
				}
				for i, c := range configs {
					fmt.Fprintf(os.Stderr, "%d) %s\n", i+1, c)
				}
				fmt.Fprint(os.Stderr, "Configuration: ")
				var n int
				if _, e = fmt.Fscanln(os.Stdin, &n); e != nil || n < 1 || n > len(configs) {
					return fmt.Errorf("invalid configuration selection")
				}
				*config = configs[n-1]
			}
		}
	}
	if *config != "" {
		v := *config
		if !filepath.IsAbs(v) {
			if _, e := os.Stat(v); e != nil {
				v = filepath.Join(local, v)
			}
		}
		var e error
		*config, e = dockerx.EnginePath(v)
		if e != nil {
			return e
		}
	}
	if command == "new" && *agent == "" && terminal() {
		fmt.Fprint(os.Stderr, "Agent [claude/codex] (claude): ")
		v, e := bufio.NewReader(os.Stdin).ReadString('\n')
		if e != nil {
			return e
		}
		*agent = strings.TrimSpace(v)
	}
	if command == "recreate" && !*yes {
		projects, e := client.Projects(ctx, &api.Empty{})
		if e != nil {
			return e
		}
		fmt.Fprintln(os.Stderr, "Recreate removes the writable layer and disconnects attached editors. Workspace and named volumes are kept.")
		for _, p := range projects.Projects {
			if p.Workspace == path || p.Name == path || p.Id == path {
				fmt.Fprintf(os.Stderr, "Target: %s %s (%s)\n", p.ContainerId, p.Workspace, p.State)
			}
		}
		if !terminal() {
			return fmt.Errorf("review targets with cxz projects, then pass --yes")
		}
		fmt.Fprint(os.Stderr, "Type recreate to continue: ")
		v, e := bufio.NewReader(os.Stdin).ReadString('\n')
		if e != nil || strings.TrimSpace(v) != "recreate" {
			return fmt.Errorf("canceled")
		}
		*yes = true
	}
	fmt.Fprintln(os.Stderr, "cxz: preparing workspace; initial image/agent downloads may take a few minutes")
	call, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	s, e := client.Open(call, &api.ProjectRequest{Workspace: path, Agent: *agent, Config: *config, NewSession: command == "new", Recreate: command == "recreate", Confirmed: *yes, TrustConfig: *trust, ClientId: core.ID()})
	if e != nil {
		return e
	}
	if *detach || !terminal() {
		return json.NewEncoder(os.Stdout).Encode(s)
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
	var matches []*api.Session
	for _, s := range list.Sessions {
		if arg == "" || s.Id == arg || strings.HasPrefix(s.Id, arg) || s.ProjectId == arg || s.Title == arg || filepath.Base(s.Workspace) == arg {
			matches = append(matches, s)
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
