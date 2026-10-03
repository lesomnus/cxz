//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/installer"
	"github.com/lesomnus/cxz/internal/transport"
	"github.com/lesomnus/xli"
)

func gitconfigCommands() *xli.Command {
	parent := commandGroup("gitconfig", "Host global Git configuration")
	parent.Commands = xli.Commands{{Name: "sync", Brief: "Refresh host Git configuration in all running projects", Handler: withClient(func(ctx context.Context, client api.SessionsClient, c *xli.Command) error {
		if _, err := transport.Load(stateFrom(ctx)); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("run cxz gitconfig sync on the installed Linux Manager host")
			}
			return err
		}
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
		if err = installer.SyncGitConfig(ctx, stateFrom(ctx), nil, running...); err != nil {
			return err
		}
		fmt.Fprintf(c.ErrWriter, "cxz: host Git configuration synced to %d running project(s); %d stopped project(s) apply at next up\n", len(running), len(list.GetProjects())-len(running))
		return nil
	})}}
	return parent
}
