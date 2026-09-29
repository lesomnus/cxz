package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/skillconfig"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

func skillCommand() *xli.Command {
	c := &xli.Command{Name: "skill", Brief: "Manage the Agent Skills library and which projects see each skill", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error { return c.PrintHelp(c.Writer) })}
	brief := map[string]string{
		"list":    "List the library and effective activation",
		"add":     "Register a skill directory from ${CXZ_SHARE_DIR}/skills",
		"remove":  "Remove a registration; the directory is kept",
		"enable":  "Enable globally by default or for one project",
		"disable": "Disable globally by default or for one project",
		"inherit": "Restore a project's global default",
	}
	for _, op := range []string{"list", "add", "remove", "enable", "disable", "inherit"} {
		command := &xli.Command{Name: op, Brief: brief[op], Flags: flg.Flags{mcpStringFlag("project", "Project ID or path (omit for global defaults)", "")}}
		if op != "list" {
			command.Args = arg.Args{mcpStringArg("NAME", false)}
		}
		command.Handler = xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			r := skillconfig.Request{Action: c.Name, Project: flg.MustGet[string](c, "project")}
			if c.Name != "list" {
				r.Name = arg.MustGet[string](c, "NAME")
			}
			session := ""
			client, closeClient, err := configConnect(ctx, c, &r.Project, &session)
			if err != nil {
				return err
			}
			defer closeClient()
			spec, err := json.Marshal(r)
			if err != nil {
				return err
			}
			out, err := client.Docker(ctx, &api.DockerInput{Action: "skills", Spec: spec})
			if err != nil {
				return err
			}
			var reply skillconfig.Reply
			if err := json.Unmarshal([]byte(out.Status), &reply); err != nil {
				return err
			}
			return printSkills(c, reply)
		})
		c.Commands = append(c.Commands, command)
	}
	return c
}

func printSkills(c *xli.Command, r skillconfig.Reply) error {
	if r.Message != "" {
		fmt.Fprintln(c.Writer, r.Message)
	}
	if len(r.Entries) == 0 {
		fmt.Fprintln(c.Writer, "No skills registered. Put a directory with SKILL.md in ${CXZ_SHARE_DIR}/skills and run cxz skill add <name>.")
		return nil
	}
	for _, e := range r.Entries {
		state := "off"
		if e.Effective {
			state = "on"
		}
		if e.Override != nil {
			state += " (project)"
		} else {
			state += " (inherited)"
		}
		fmt.Fprintf(c.Writer, "%-24s %-16s %s\n", e.Name, state, e.Description)
	}
	return nil
}
