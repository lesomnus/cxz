//go:build !windows

package main

import (
	"context"
	"fmt"
	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
	"time"
)

func sessionEraseCommand() *xli.Command {
	c := &xli.Command{
		Name: "erase", Brief: "Permanently delete a session and its stored data",
		Flags: flg.Flags{switchFlag("yes", "Confirm irreversible deletion of this session's data")},
		Args:  arg.Args{mcpStringArg("SESSION", false)},
	}
	c.Handler = xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		if !flg.MustGet[bool](c, "yes") {
			return fmt.Errorf("session erase permanently deletes its history, profile, memory and attachments; pass --yes to confirm")
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		session := arg.MustGet[string](c, "SESSION")
		project := ""
		client, closeClient, err := configConnect(ctx, c, &project, &session)
		if err != nil {
			return err
		}
		defer closeClient()
		out, err := client.EraseSession(ctx, &api.SessionEraseInput{SessionId: session})
		if err != nil {
			return err
		}
		if format, set := flg.Get[string](c, "format"); set && format == "json" {
			return writeJSON(c.Writer, out)
		}
		_, err = fmt.Fprintln(c.Writer, "Erased session", session)
		return err
	})
	return c
}
