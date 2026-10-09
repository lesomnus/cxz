package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lesomnus/cxz/api"
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
			project := flg.MustGet[string](c, "project")
			name := ""
			if c.Name != "list" {
				name = arg.MustGet[string](c, "NAME")
			}
			if c.Name == "inherit" && project == "" {
				return fmt.Errorf("inherit needs a project to restore")
			}
			session := ""
			client, closeClient, err := configConnect(ctx, c, &project, &session)
			if err != nil {
				return err
			}
			defer closeClient()
			reply, err := callSkill(ctx, client, c.Name, project, name)
			if err != nil {
				return err
			}
			return printSkills(c, reply)
		})
		c.Commands = append(c.Commands, command)
	}
	return c
}

// Each verb is its own call, and enable and disable split by scope rather than
// by name: setting the installation's default and deciding for one project are
// different decisions written to different places.
func callSkill(ctx context.Context, client api.SessionsClient, op, project, name string) (*api.SkillsReply, error) {
	switch op {
	case "list":
		return client.GetSkills(ctx, &api.SkillsInput{Project: project})
	case "add":
		return client.AddSkill(ctx, &api.SkillInput{Project: project, Name: name})
	case "remove":
		return client.RemoveSkill(ctx, &api.SkillInput{Project: project, Name: name})
	case "enable", "disable":
		on := op == "enable"
		if project == "" {
			return client.SetSkillDefault(ctx, &api.SkillDefaultInput{Name: name, Enabled: on})
		}
		return client.SetProjectSkill(ctx, &api.ProjectSkillInput{Project: project, Name: name, Enabled: on})
	case "inherit":
		return client.ClearProjectSkill(ctx, &api.ClearProjectSkillInput{Project: project, Name: name})
	}
	return nil, fmt.Errorf("unknown skill command: %s", op)
}

func printSkills(c *xli.Command, r *api.SkillsReply) error {
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
