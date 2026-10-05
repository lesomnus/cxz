package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/auxiliary"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"time"
)

func aiCommand() *xli.Command {
	cmd := &xli.Command{Name: "ai", Brief: "Configure auxiliary summaries, suggestions and session titles", Handler: xli.OnRun(func(_ context.Context, c *xli.Command, _ xli.Next) error { return c.PrintHelp(c.Writer) })}
	for _, op := range []string{"list", "set", "disable", "models", "login", "status", "cancel", "title"} {
		c := &xli.Command{Name: op}
		if op != "list" {
			c.Args = arg.Args{mcpStringArg("TARGET", false)}
		}
		if op == "title" {
			c.Flags = flg.Flags{mcpStringFlag("text", "Manual title (empty: generate)", "")}
		}
		if op == "set" {
			c.Flags = flg.Flags{mcpStringFlag("account", "Registered account", ""), mcpStringFlag("model", "Provider model", ""), mcpStringFlag("effort", "Provider effort (empty: default)", "")}
		}
		c.Handler = xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
			q := auxiliary.Request{Action: c.Name}
			target := ""
			if c.Name != "list" {
				target = arg.MustGet[string](c, "TARGET")
			}
			switch c.Name {
			case "set":
				q.Action = "put"
				q.Task = target
				q.Profile = auxiliary.Profile{Enabled: true, Account: flg.MustGet[string](c, "account"), Model: flg.MustGet[string](c, "model"), Effort: flg.MustGet[string](c, "effort")}
			case "disable":
				q.Action = "put"
				q.Task = target
			case "models":
				q.Profile.Account = target
			case "login":
				q.Action = "login-info"
				q.Profile.Account = target
			case "title":
				q.Session = target
				q.Text = flg.MustGet[string](c, "text")
			case "status", "cancel":
				q.Session = target
			}
			request, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			project := ""
			client, closeClient, e := configConnect(request, c, &project, &q.Session)
			if e != nil {
				return e
			}
			defer closeClient()
			b, _ := json.Marshal(q)
			r, e := client.Docker(request, &api.DockerInput{Action: "auxiliary", Spec: b})
			if e != nil {
				return e
			}
			var out auxiliary.Reply
			if e = json.Unmarshal([]byte(r.Status), &out); e != nil {
				return e
			}
			if c.Name == "login" {
				if out.Profile == nil {
					return fmt.Errorf("manager omitted auxiliary account")
				}
				return aiLogin(ctx, c, out)
			}
			enc := json.NewEncoder(c.Writer)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		})
		cmd.Commands = append(cmd.Commands, c)
	}
	return cmd
}
