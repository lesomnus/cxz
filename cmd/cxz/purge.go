//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lesomnus/cxz/internal/purge"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"
)

func purgeCommand() *xli.Command {
	return &xli.Command{Name: "purge", Brief: "Select and permanently delete this installation's cxz data", Flags: flg.Flags{switchFlag("dry-run", "List exact cleanup targets without changing anything"), switchFlag("yes", "Confirm permanent deletion of selected data"), stringFlag("groups", "Comma-separated cleanup groups: containers,projects,state,tools,networks,local", "containers,projects,state,tools,networks,local")}, Handler: onRun(func(ctx context.Context, c *xli.Command) error {
		if os.Getenv("CXZ_OWNER") != "" || os.Getenv("CXZ_PROJECT_ID") != "" {
			return fmt.Errorf("run purge from the host/client, not inside the manager or a project")
		}
		dry := flg.MustGet[bool](c, "dry-run")
		if !dry && !flg.MustGet[bool](c, "yes") {
			return fmt.Errorf("purge requires --yes; use --dry-run to inspect targets")
		}
		selected := map[string]bool{}
		for _, name := range strings.Split(flg.MustGet[string](c, "groups"), ",") {
			name = strings.TrimSpace(name)
			valid := false
			for _, g := range purge.Groups {
				if g.ID == name {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("unknown purge group %q", name)
			}
			selected[name] = true
		}
		p, err := purge.Discover(ctx, stateFrom(ctx), nil)
		if err != nil {
			return err
		}
		if dry {
			fmt.Fprintln(c.Writer, "State:", p.Root)
			for _, g := range purge.Groups {
				if !selected[g.ID] {
					continue
				}
				fmt.Fprintln(c.Writer, g.Label)
				for _, t := range p.Targets {
					if t.Group == g.ID {
						fmt.Fprintln(c.Writer, "  ", t.Kind, t.Name)
					}
				}
			}
			for _, name := range p.Preserved {
				fmt.Fprintln(c.Writer, "Preserved unknown entry:", name)
			}
			fmt.Fprintln(c.Writer, "Workspace sources, personal agent logins, shared images/build cache and CLI binary are never deleted.")
			return nil
		}
		if err = purge.Execute(ctx, p, selected, nil, c.Writer); err != nil {
			return err
		}
		fmt.Fprintln(c.Writer, "Selected cxz data deleted. Deleted data requires backups to recover.")
		for _, name := range p.Preserved {
			fmt.Fprintln(c.Writer, "Preserved unknown entry:", name)
		}
		return nil
	})}
}
