package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"io"
	"time"
)

// Indented JSON, which is what this command has always written. It cannot share
// the CLI's own writer: that one is built for the platforms the rest of the
// commands are built for, and cxz ai is on Windows too.
func writeAux(w io.Writer, v proto.Message) error {
	b, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

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
			target := ""
			if c.Name != "list" {
				target = arg.MustGet[string](c, "TARGET")
			}
			// Only three of these name a session. The rest name a kind or an
			// account and go to the connection the caller is looking at.
			session := ""
			switch c.Name {
			case "status", "cancel", "title":
				session = target
			}
			request, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			project := ""
			client, closeClient, err := configConnect(request, c, &project, &session)
			if err != nil {
				return err
			}
			defer closeClient()

			var out proto.Message
			switch c.Name {
			case "list":
				out, err = client.AuxConfig(request, &api.Empty{})
			case "set":
				out, err = client.AuxSetConfig(request, &api.AuxSetConfigInput{Profiles: []*api.AuxProfile{{
					Kind: target, Enabled: true,
					Account: flg.MustGet[string](c, "account"),
					Model:   flg.MustGet[string](c, "model"),
					Effort:  flg.MustGet[string](c, "effort"),
				}}})
			case "disable":
				out, err = client.AuxSetConfig(request, &api.AuxSetConfigInput{Profiles: []*api.AuxProfile{{Kind: target}}})
			case "models":
				out, err = client.AuxModels(request, &api.AuxModelsInput{Account: target})
			case "login":
				info, e := client.AuxLoginInfo(request, &api.AuxLoginInfoInput{Account: target})
				if e != nil {
					return e
				}
				if info.Account == "" {
					return fmt.Errorf("manager omitted auxiliary account")
				}
				return aiLogin(ctx, c, info)
			case "status":
				out, err = client.AuxStatus(request, &api.AuxStatusInput{SessionId: session})
			case "cancel":
				out, err = client.AuxCancel(request, &api.AuxCancelInput{SessionId: session})
			case "title":
				out, err = client.AuxRun(request, &api.AuxRunInput{
					SessionId: session, Kinds: []string{"title"}, Text: flg.MustGet[string](c, "text"),
				})
			}
			if err != nil {
				return err
			}
			return writeAux(c.Writer, out)
		})
		cmd.Commands = append(cmd.Commands, c)
	}
	return cmd
}
