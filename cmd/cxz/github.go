//go:build !windows

package main

import (
	"context"
	"fmt"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/xli"
)

func githubCommands() *xli.Command {
	parent := &xli.Command{Name: "github", Brief: "Host GitHub CLI credentials", Handler: onRun(func(_ context.Context, c *xli.Command) error { return c.PrintHelp(c.Writer) })}
	parent.Commands = xli.Commands{
		{Name: "sync", Brief: "Copy the current host gh credentials into every running project", Handler: withClient(githubSync)},
	}
	return parent
}

// One injection per project reaches all of its sessions: GH_CONFIG_DIR is a
// project path, not a session one, so the separated session HOMEs share it.
// Adding a scope on the host and syncing keeps one token where a login inside
// each project would mint another and push older ones past GitHub's limit.
func githubSync(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
	list, err := client.Projects(ctx, &api.Empty{})
	if err != nil {
		return err
	}
	var running []*api.Project
	for _, p := range list.GetProjects() {
		if p.State == "running" {
			running = append(running, p)
		}
	}
	// The manager snapshot is written even with no running project; stopped ones
	// take their copy from it when they are next provisioned.
	if err := installer.SyncGitHub(ctx, stateFrom(ctx), nil, running...); err != nil {
		return err
	}
	rest := len(list.GetProjects()) - len(running)
	line := fmt.Sprintf("cxz: host gh credentials synced to %d running project(s)", len(running))
	if rest > 0 {
		line += fmt.Sprintf("; %d more apply at their next up", rest)
	}
	fmt.Fprintln(c.ErrWriter, line)
	return nil
}
