package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/mcpconfig"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
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
			project, session := flg.MustGet[string](c, "project"), flg.MustGet[string](c, "session")
			id := ""
			if c.Name != "list" {
				id = arg.MustGet[string](c, "ID")
			}
			var server *api.McpServer
			switch c.Name {
			case "add":
				v, e := readMCPDefinition(arg.MustGet[string](c, "FILE"))
				if e != nil {
					return e
				}
				server = v
			case "inherit":
				if project == "" {
					return fmt.Errorf("inherit requires --project")
				}
			}
			client, closeClient, e := configConnect(ctx, c, &project, &session)
			if e != nil {
				return e
			}
			defer closeClient()
			out, e := callMCP(ctx, client, c.Name, project, session, id, server)
			if e != nil {
				return e
			}
			return mcpOutput(c, out)
		})
		c.Commands = append(c.Commands, command)
	}
	return c
}

// mcpView keeps the shape of --format json output. It is this command's
// interface rather than a wire shape, which is why it lives here now: the
// envelope's reply carried all four fields at once whatever was asked.
type mcpEntryView struct {
	ID        string           `json:"id"`
	Server    mcpconfig.Server `json:"server"`
	Override  *bool            `json:"override,omitempty"`
	Effective bool             `json:"effective"`
}
type mcpSessionView struct {
	LaunchDigest string            `json:"launch_digest,omitempty"`
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Pending      bool              `json:"pending"`
	Servers      map[string]string `json:"servers"`
}
type mcpView struct {
	Log      string           `json:"log,omitempty"`
	Sessions []mcpSessionView `json:"sessions,omitempty"`
	Entries  []mcpEntryView   `json:"entries"`
	Message  string           `json:"message,omitempty"`
}

// Each verb is its own call, and enable and disable split by scope rather than
// by name: the installation's default and one project's override are different
// decisions written to different places. logs and restart are the session's.
func callMCP(ctx context.Context, client api.SessionsClient, op, project, session, id string, server *api.McpServer) (mcpView, error) {
	switch op {
	case "logs":
		out, err := client.McpLogs(ctx, &api.McpLogsInput{SessionId: session, Id: id})
		if err != nil {
			return mcpView{}, err
		}
		return mcpView{Log: out.Text, Message: out.Message, Entries: []mcpEntryView{}}, nil
	case "restart":
		out, err := client.RestartMcp(ctx, &api.RestartMcpInput{SessionId: session, Id: id})
		if err != nil {
			return mcpView{}, err
		}
		return mcpView{Message: out.Status, Entries: []mcpEntryView{}}, nil
	}
	reply, err := mcpSettings(ctx, client, op, project, id, server)
	if err != nil {
		return mcpView{}, err
	}
	out := mcpView{Message: reply.GetMessage(), Entries: []mcpEntryView{}}
	for _, e := range reply.GetEntries() {
		s := e.GetServer()
		out.Entries = append(out.Entries, mcpEntryView{
			ID: e.GetId(), Effective: e.GetEffective(), Override: e.Override,
			Server: mcpconfig.Server{
				Name: s.GetName(), Kind: s.GetKind(), Enabled: s.GetEnabled(),
				Command: s.GetCommand(), Args: s.GetArgs(), Env: s.GetEnv(),
				URL: s.GetUrl(), Headers: s.GetHeaders(),
			},
		})
	}
	// What each live session launched with is a second question, asked only
	// when a project was named -- as it was before, except that a container
	// that cannot answer no longer delays reading the registrations.
	if project == "" {
		return out, nil
	}
	status, err := client.McpSessions(ctx, &api.McpSessionsInput{Project: project})
	if err != nil {
		out.Message += " Runtime status unavailable."
		return out, nil
	}
	if status.Message != "" {
		out.Message += " " + status.Message
	}
	for _, v := range status.Sessions {
		out.Sessions = append(out.Sessions, mcpSessionView{
			LaunchDigest: v.LaunchDigest, ID: v.SessionId, Title: v.Title,
			Pending: v.Pending, Servers: v.Servers,
		})
	}
	return out, nil
}

func mcpSettings(ctx context.Context, client api.SessionsClient, op, project, id string, server *api.McpServer) (*api.McpServersReply, error) {
	switch op {
	case "list":
		return client.GetMcpServers(ctx, &api.McpServersInput{Project: project})
	case "add":
		return client.PutMcpServer(ctx, &api.PutMcpServerInput{Id: id, Server: server})
	case "remove":
		return client.RemoveMcpServer(ctx, &api.McpServerInput{Id: id})
	case "enable", "disable":
		on := op == "enable"
		if project == "" {
			return client.SetMcpServerDefault(ctx, &api.McpServerDefaultInput{Id: id, Enabled: on})
		}
		return client.SetProjectMcpServer(ctx, &api.ProjectMcpServerInput{Project: project, Id: id, Enabled: on})
	case "inherit":
		return client.ClearProjectMcpServer(ctx, &api.ClearProjectMcpServerInput{Project: project, Id: id})
	}
	return nil, fmt.Errorf("unknown MCP command: %s", op)
}

// A definition is read from a file rather than from flags, so an environment
// value never appears in a shell history or a process listing.
func readMCPDefinition(path string) (*api.McpServer, error) {
	file, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer file.Close()
	b, e := io.ReadAll(io.LimitReader(file, 256*1024+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 256*1024 {
		return nil, fmt.Errorf("MCP definition exceeds 256 KiB")
	}
	var s mcpconfig.Server
	if e = json.Unmarshal(b, &s); e != nil {
		return nil, e
	}
	return &api.McpServer{
		Name: s.Name, Kind: s.Kind, Enabled: s.Enabled,
		Command: s.Command, Args: s.Args, Env: s.Env,
		Url: s.URL, Headers: s.Headers,
	}, nil
}

func mcpStringFlag(name, brief, value string) *flg.String {
	return &flg.String{Name: name, Brief: brief, Default: &value}
}
func mcpStringArg(name string, optional bool) *arg.String {
	return &arg.String{Name: name, Optional: optional}
}
