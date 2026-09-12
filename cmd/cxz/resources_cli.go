package main

import (
	"context"

	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
)

func commandGroup(name, brief string) *xli.Command {
	return &xli.Command{Name: name, Brief: brief, Handler: onRun(func(_ context.Context, c *xli.Command) error { return c.PrintHelp(c.Writer) })}
}

// Public commands have one canonical resource/verb path. Internal runtime
// entrypoints deliberately retain their wire/process contract.
func reorganizeCommands(root *xli.Command) {
	root.Flags = append(root.Flags, formatFlag())
	project, session, manager := commandGroup("project", "Manage owned workspaces"), commandGroup("session", "Manage agent conversation sessions"), commandGroup("manager", "Manage the background server")
	var keep xli.Commands
	for _, c := range root.Commands {
		switch c.Name {
		case "project":
			project.Commands = append(project.Commands, c.Commands...)
		case "up", "down", "recreate", "shell", "exec":
			project.Commands = append(project.Commands, c)
		case "projects":
			c.Name = "ls"
			c.Brief = "List owned and foreign projects"
			project.Commands = append(project.Commands, c)
		case "new", "ls", "get", "send", "reply", "interrupt", "resume", "stop", "events", "attach":
			c.Aliases = nil
			if c.Name == "ls" {
				c.Brief = "List sessions"
			}
			session.Commands = append(session.Commands, c)
		case "install", "uninstall", "serve", "update", "rollback", "doctor":
			manager.Commands = append(manager.Commands, c)
		case "logs":
			c.Args = nil
			c.Brief = "Show manager logs"
			manager.Commands = append(manager.Commands, c)
			logs := logsCommand()
			logs.Args = arg.Args{stringArg("PROJECT", false)}
			logs.Brief = "Show project provisioning logs"
			project.Commands = append(project.Commands, logs)
		case "config":
			show := &xli.Command{Name: "show", Brief: c.Brief, Handler: c.Handler}
			c.Handler = commandGroup("config", "").Handler
			c.Commands = append(c.Commands, show)
			keep = append(keep, c)
		case "account":
			var commands xli.Commands
			for _, sub := range c.Commands {
				switch sub.Name {
				case "list":
					sub.Name = "ls"
					commands = append(commands, sub)
				case "backends":
					sub.Name = "ls"
					g := commandGroup("backend", "Inspect authentication strategies")
					g.Commands = xli.Commands{sub}
					keep = append(keep, g)
				case "bindings":
					sub.Name = "ls"
					g := commandGroup("binding", "Inspect account/project authentication bindings")
					g.Commands = xli.Commands{sub}
					keep = append(keep, g)
				default:
					commands = append(commands, sub)
				}
			}
			c.Commands = commands
			keep = append(keep, c)
		default:
			keep = append(keep, c)
		}
	}
	root.Commands = append(keep, project, session, manager)
	root.Synop = "Flags precede positional arguments: cxz account add codex work; cxz session new --account work .\nUse --format json on data commands for scripts. Ctrl-C detaches the TUI."
	// Parent pointers are attached by xli later; traverse with explicit scope.
	for _, g := range root.Commands {
		if g.Name == "version" {
			g.Flags = append(g.Flags, formatFlag())
		}
		for _, c := range g.Commands {
			data := g.Name == "config" || g.Name == "backend" || g.Name == "binding" ||
				(g.Name == "account" && c.Name != "login") ||
				(g.Name == "project" && c.Name != "logs" && c.Name != "shell" && c.Name != "exec") ||
				(g.Name == "session" && c.Name != "attach") || (g.Name == "manager" && c.Name == "doctor")
			if data {
				c.Flags = append(c.Flags, formatFlag())
			}
		}
	}
}
