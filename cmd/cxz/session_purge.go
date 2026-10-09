//go:build !windows

package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lesomnus/cxz/api"
	"github.com/lesomnus/cxz/internal/sessionpurge"
	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

func sessionPurgeCommand() *xli.Command {
	c := &xli.Command{
		Name:  "purge",
		Brief: "Permanently delete a session and everything it stored",
		Synop: "Deleting a session keeps its journal, so the delete stays recoverable; purge does not.\nRun --dry-run first: it reports the exact bytes at stake and changes nothing.",
		Flags: flg.Flags{
			switchFlag("dry-run", "Report what purge would delete without changing anything"),
			switchFlag("yes", "Confirm irreversible deletion of this session's data"),
		},
		Args: arg.Args{mcpStringArg("SESSION", false)},
	}
	c.Handler = xli.OnRun(func(ctx context.Context, c *xli.Command, _ xli.Next) error {
		dry := flg.MustGet[bool](c, "dry-run")
		if !dry && !flg.MustGet[bool](c, "yes") {
			return fmt.Errorf("purge requires --yes; use --dry-run to inspect what it would delete")
		}
		// Purging stops the agent and waits for it, so allow more than the usual
		// half minute before giving up on a session mid-turn.
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		session := arg.MustGet[string](c, "SESSION")
		project := ""
		client, closeClient, err := configConnect(ctx, c, &project, &session)
		if err != nil {
			return err
		}
		defer closeClient()
		out, err := client.PurgeSession(ctx, &api.SessionPurgeInput{SessionId: session, DryRun: dry})
		if err != nil {
			return err
		}
		reply := sessionpurge.Reply{Session: out.SessionId, DryRun: out.DryRun, Retained: out.Retained}
		for _, t := range out.Targets {
			reply.Targets = append(reply.Targets, sessionpurge.Target{
				Kind: t.Kind, Path: t.Path, Files: int(t.Files), Bytes: t.Bytes,
			})
		}
		return printSessionPurge(c, reply)
	})
	return c
}

var purgeKinds = map[string]string{
	"journal": "conversation journal and session logs",
	"profile": "agent profile, credentials and its own transcript",
	"memory":  "memory documents this session published",
	"uploads": "files attached to this session",
	"socket":  "supervisor socket",
	"record":  "session record, title and alias",
}

func printSessionPurge(c *xli.Command, r sessionpurge.Reply) error {
	if format, set := flg.Get[string](c, "format"); set && format == "json" {
		return writeJSON(c.Writer, r)
	}
	verb := "Deleted"
	if r.DryRun {
		verb = "Would delete"
	}
	if len(r.Targets) == 0 {
		fmt.Fprintln(c.Writer, "Nothing left to purge for", r.Session)
		return nil
	}
	targets := append([]sessionpurge.Target(nil), r.Targets...)
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].Bytes > targets[j].Bytes })
	fmt.Fprintf(c.Writer, "%s for %s:\n", verb, r.Session)
	for _, t := range targets {
		label := purgeKinds[t.Kind]
		if label == "" {
			label = t.Kind
		}
		fmt.Fprintf(c.Writer, "  %-12s %9s  %s\n", t.Kind, bytesLabel(t.Bytes), label)
	}
	fmt.Fprintln(c.Writer, "Total:", bytesLabel(r.Bytes()))
	for _, name := range r.Retained {
		fmt.Fprintln(c.Writer, "Retained:", name)
	}
	if !r.DryRun {
		fmt.Fprintln(c.Writer, "Purged data requires backups to recover.")
	}
	return nil
}

func bytesLabel(n int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	value := float64(n)
	for i, unit := range units {
		if value < 1024 || i == len(units)-1 {
			if i == 0 {
				return fmt.Sprintf("%d %s", n, unit)
			}
			return fmt.Sprintf("%.1f %s", value, unit)
		}
		value /= 1024
	}
	return fmt.Sprint(n)
}
