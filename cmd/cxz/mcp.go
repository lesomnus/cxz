package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"io"
	"os"
	"time"
)

func mcpCommand() *xli.Command {
	c := &xli.Command{Name: "mcp", Brief: "Manage MCP servers and project activation (no agent restart)", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error { return c.PrintHelp(c.Writer) })}
	for _, op := range []string{"list", "add", "remove", "enable", "disable", "inherit", "logs", "restart"} {
		command := &xli.Command{Name: op, Brief: map[string]string{"list": "List registrations and effective activation", "add": "Register or replace an external MCP from a private JSON file", "remove": "Remove an external MCP registration", "enable": "Enable globally by default or for one project", "disable": "Disable globally by default or for one project", "logs": "Read the selected session MCP stderr", "restart": "Reconnect the selected session MCP without restarting its agent", "inherit": "Restore a project's global default"}[op], Flags: flg.Flags{mcpStringFlag("project", "Project ID or path (omit for global defaults)", ""), mcpStringFlag("session", "Session ID for logs/restart", "")}}
		if op != "list" {
			command.Args = arg.Args{mcpStringArg("ID", false)}
		}
		if op == "add" {
			command.Args = append(command.Args, mcpStringArg("FILE", false))
		}
		command.Handler = xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			r := mcpconfig.Request{Session: flg.MustGet[string](c, "session"), Action: c.Name, Project: flg.MustGet[string](c, "project")}
			if c.Name != "list" {
				r.ID = arg.MustGet[string](c, "ID")
			}
			switch c.Name {
			case "add":
				file, e := os.Open(arg.MustGet[string](c, "FILE"))
				if e != nil {
					return e
				}
				defer file.Close()
				b, e := io.ReadAll(io.LimitReader(file, 256*1024+1))
				if e != nil {
					return e
				}
				if len(b) > 256*1024 {
					return fmt.Errorf("MCP definition exceeds 256 KiB")
				}
				var s mcpconfig.Server
				if e = json.Unmarshal(b, &s); e != nil {
					return e
				}
				r.Server = &s
				r.Action = "put"
			case "enable", "disable":
				v := c.Name == "enable"
				r.Enabled = &v
				r.Action = "enable"
			case "inherit":
				if r.Project == "" {
					return fmt.Errorf("inherit requires --project")
				}
				r.Action = "enable"
			}
			client, closeClient, e := configConnect(ctx, c, &r.Project, &r.Session)
			if e != nil {
				return e
			}
			defer closeClient()
			b, _ := json.Marshal(r)
			out, e := client.Docker(ctx, &api.DockerInput{Action: "mcp", Spec: b})
			if e != nil {
				return e
			}
			var reply mcpconfig.Reply
			if e = json.Unmarshal([]byte(out.Status), &reply); e != nil {
				return e
			}
			return mcpOutput(c, reply)
		})
		c.Commands = append(c.Commands, command)
	}
	return c
}

func mcpStringFlag(name, brief, value string) *flg.String {
	return &flg.String{Name: name, Brief: brief, Default: &value}
}
func mcpStringArg(name string, optional bool) *arg.String {
	return &arg.String{Name: name, Optional: optional}
}
