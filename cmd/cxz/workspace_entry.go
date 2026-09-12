package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/core"
	"github.com/lesomnus/cxz/internal/dockerx"
	"github.com/lesomnus/cxz/internal/resourceclient"
	"github.com/lesomnus/cxz/internal/settings"
	"github.com/lesomnus/cxz/internal/tui"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

func workspaceEntryCommands() xli.Commands {
	up := newProjectCommand("up")
	up.Brief = "Prepare workspace and open its project dashboard (no session created)"
	var flags flg.Flags
	for _, f := range up.Flags {
		if f.Info().Name == "no-attach" {
			f.(*flg.Switch).Brief = "Print project data without opening TUI (see --format)"
		}
		if f.Info().Name != "account" && f.Info().Name != "model" {
			flags = append(flags, f)
		}
	}
	up.Flags = append(flags, formatFlag())
	down := &xli.Command{Name: "down", Brief: "Delete workspace project and sessions; retain source and named volumes", Args: arg.Args{projectArg("WORKSPACE", true)}, Flags: flg.Flags{formatFlag()}, Handler: withClient(workspaceEntry)}
	it := &xli.Command{Name: "it", Brief: "Open the workspace's newest session (does not create one)", Args: arg.Args{projectArg("WORKSPACE", true)}, Handler: withClient(workspaceEntry)}
	down.Args[0].(*arg.String).Default = ptr(".")
	it.Args[0].(*arg.String).Default = ptr(".")
	return xli.Commands{up, down, it}
}

func workspaceEntry(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	path := arg.MustGet[string](c, "WORKSPACE")
	if path == "" {
		path = "."
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		var e error
		path, e = dockerx.EnginePath(path)
		if e != nil {
			return e
		}
	}
	resources := client.(*resourceclient.Client)
	p, err := resources.ResolveProject(ctx, path)
	if err != nil {
		return err
	}
	if c.Name == "down" {
		if err := resources.DeleteProject(ctx, p.Id); err != nil {
			return err
		}
		return writeOutput(c, map[string]any{"project": p.Id, "status": "deleted", "retained": "workspace source, named volumes and archived journals"})
	}
	if !terminal(c) {
		return fmt.Errorf("it requires an interactive terminal; use cxz session get SESSION for data")
	}
	list, err := resources.List(ctx, &api.Empty{})
	if err != nil {
		return err
	}
	sessions := tui.ProjectSessions(list.Sessions, p)
	if len(sessions) == 0 {
		return fmt.Errorf("project has no sessions; run cxz up to create one from its dashboard")
	}
	return projectTUI(ctx, resources, c, p, sessions[0].Id, false)
}

func projectTUI(ctx context.Context, resources *resourceclient.Client, c *xli.Command, p *api.Project, selected string, trust bool) error {
	return tui.RunProject(ctx, resources, p, selected, func(input io.Reader, output, errOutput io.Writer) (*api.Session, error) {
		// Bubble Tea has restored the terminal before this callback. Account
		// selection and official OAuth may temporarily own it, then return here.
		oldIn, oldOut, oldErr := c.ReadCloser, c.Writer, c.ErrWriter
		c.ReadCloser = io.NopCloser(input)
		if f, ok := input.(io.ReadCloser); ok {
			c.ReadCloser = f
		}
		c.Writer, c.ErrWriter = output, errOutput
		defer func() { c.ReadCloser, c.Writer, c.ErrWriter = oldIn, oldOut, oldErr }()
		alias, err := selectProjectAccount(ctx, resources, c)
		if err != nil {
			return nil, err
		}
		a, err := resources.Account(ctx, alias)
		if err != nil {
			return nil, err
		}
		r := &api.ProjectRequest{Workspace: p.Id, NewSession: true, Account: alias, Agent: a.GetAgent(), Model: settings.From(ctx).Model(a.GetAgent()), TrustConfig: trust, ClientId: core.ID()}
		return openWithProjectLogin(ctx, r, true, func(ctx context.Context, r *api.ProjectRequest) (*api.Session, error) { return resources.Open(ctx, r) }, func(ctx context.Context, alias string) error {
			return projectAccountWorkflow(ctx, resources, c, a, "login", p.Id, false)
		})
	})
}
